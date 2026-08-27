package service

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var (
	ErrOrganizationMemberQueryInvalid = errors.New("invalid organization member query")
	ErrOrganizationPolicyInvalid      = errors.New("invalid organization top-up policy")
)

type ListOrganizationMembersParams struct {
	Offset  int
	Limit   int
	Status  *model.OrganizationMemberStatus
	Keyword string
}

// OrganizationMemberView is a safe administrative projection. It exposes the
// user's single wallet and the recoverable organization-funded portion, while
// never serializing credentials or management tokens.
type OrganizationMemberView struct {
	UserID             int                            `json:"user_id"`
	Username           string                         `json:"username"`
	DisplayName        string                         `json:"display_name"`
	Email              string                         `json:"email"`
	PlatformRole       int                            `json:"platform_role"`
	OrganizationID     int                            `json:"organization_id"`
	OrganizationRole   model.OrganizationRole         `json:"organization_role"`
	OrganizationStatus model.OrganizationMemberStatus `json:"organization_status"`
	Quota              int                            `json:"quota"`
	UsedQuota          int                            `json:"used_quota"`
	RequestCount       int                            `json:"request_count"`
	RecoverableQuota   int64                          `json:"recoverable_quota"`
	ConsumedQuota      int64                          `json:"consumed_quota"`
	ConsumptionLimit   *int64                         `json:"consumption_limit,omitempty"`
	CreatedAt          int64                          `json:"created_at"`
}

type OrganizationMemberListResult struct {
	Items []OrganizationMemberView `json:"items"`
	Total int64                    `json:"total"`
}

type UpdateOrganizationTopupPolicyParams struct {
	OrganizationID   int
	AllowMemberTopup bool
	RequestID        string
}

func normalizeOrganizationMemberStatus(status model.OrganizationMemberStatus) (model.OrganizationMemberStatus, error) {
	status = model.OrganizationMemberStatus(strings.ToLower(strings.TrimSpace(string(status))))
	switch status {
	case model.OrganizationMemberStatusActive, model.OrganizationMemberStatusDisabled:
		return status, nil
	default:
		return "", ErrOrganizationMemberQueryInvalid
	}
}

func validateOrganizationMemberListParams(params *ListOrganizationMembersParams) error {
	if params == nil {
		return ErrOrganizationMemberQueryInvalid
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	if params.Limit <= 0 {
		params.Limit = common.ItemsPerPage
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	params.Keyword = strings.TrimSpace(params.Keyword)
	if len(params.Keyword) > 128 {
		return ErrOrganizationMemberQueryInvalid
	}
	if params.Status != nil {
		status, err := normalizeOrganizationMemberStatus(*params.Status)
		if err != nil {
			return err
		}
		params.Status = &status
	}
	return nil
}

func validatePlatformOrganizationScope(actorUserID, organizationID int) error {
	if actorUserID <= 0 || organizationID <= 0 || model.DB == nil {
		return ErrPlatformProvisioningForbidden
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := loadPlatformOrganizationActorTx(tx, actorUserID, false); err != nil {
			return err
		}
		var organization model.Organization
		return tx.Select("id").Where("id = ?", organizationID).First(&organization).Error
	})
}

// ListOrganizationMembersForPlatform returns members for one organization;
// it is platform-only for now and therefore cannot be used by a tenant Owner
// or Admin as a cross-organization enumeration primitive.
func ListOrganizationMembersForPlatform(actorUserID, organizationID int, params ListOrganizationMembersParams) (*OrganizationMemberListResult, error) {
	if err := validateOrganizationMemberListParams(&params); err != nil {
		return nil, err
	}
	if err := validatePlatformOrganizationScope(actorUserID, organizationID); err != nil {
		return nil, err
	}
	query := model.DB.Model(&model.User{}).Where("organization_id = ?", organizationID)
	if params.Status != nil {
		query = query.Where("organization_status = ?", *params.Status)
	}
	if params.Keyword != "" {
		like := "%" + params.Keyword + "%"
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var users []model.User
	if err := query.Omit("password", "access_token").Order("id asc").Offset(params.Offset).Limit(params.Limit).Find(&users).Error; err != nil {
		return nil, err
	}

	funds := make(map[int]model.OrganizationMemberFund, len(users))
	if len(users) > 0 {
		ids := make([]int, 0, len(users))
		for _, user := range users {
			ids = append(ids, user.Id)
		}
		var memberFunds []model.OrganizationMemberFund
		if err := model.DB.Where("organization_id = ? AND user_id IN ?", organizationID, ids).Find(&memberFunds).Error; err != nil {
			return nil, err
		}
		for _, fund := range memberFunds {
			funds[fund.UserId] = fund
		}
	}
	items := make([]OrganizationMemberView, 0, len(users))
	for _, user := range users {
		view := OrganizationMemberView{
			UserID:             user.Id,
			Username:           user.Username,
			DisplayName:        user.DisplayName,
			Email:              user.Email,
			PlatformRole:       user.Role,
			OrganizationID:     user.OrganizationId,
			OrganizationRole:   user.OrganizationRole,
			OrganizationStatus: user.OrganizationStatus,
			Quota:              user.Quota,
			UsedQuota:          user.UsedQuota,
			RequestCount:       user.RequestCount,
			CreatedAt:          user.CreatedAt,
		}
		if fund, ok := funds[user.Id]; ok {
			view.RecoverableQuota = fund.RecoverableQuota
			view.ConsumedQuota = fund.ConsumedQuota
			view.ConsumptionLimit = fund.ConsumptionLimit
		}
		items = append(items, view)
	}
	return &OrganizationMemberListResult{Items: items, Total: total}, nil
}

// UpdateOrganizationTopupPolicyForPlatform changes whether members may pay
// for their own wallet. Because this policy affects authorization decisions,
// all member auth versions are advanced in the same transaction and sessions
// are revoked after commit.
func UpdateOrganizationTopupPolicyForPlatform(actorUserID int, params UpdateOrganizationTopupPolicyParams) (*model.Organization, error) {
	if params.OrganizationID <= 0 || model.DB == nil {
		return nil, ErrOrganizationPolicyInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	var organization model.Organization
	changes := make([]model.OrganizationAuthorizationChange, 0)
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization = *current
		if organization.Status == model.OrganizationStatusDissolved {
			return model.ErrOrganizationNotActive
		}
		if organization.AllowMemberTopup == params.AllowMemberTopup {
			return nil
		}
		var userIDs []int
		if err := tx.Model(&model.User{}).Where("organization_id = ?", organization.Id).Order("id asc").Pluck("id", &userIDs).Error; err != nil {
			return err
		}
		nextPolicyVersion := organization.PolicyVersion
		if nextPolicyVersion < 1 {
			nextPolicyVersion = 1
		}
		if nextPolicyVersion == math.MaxInt64 {
			return ErrOrganizationPolicyInvalid
		}
		nextPolicyVersion++
		if err := tx.Model(&model.Organization{}).Where("id = ?", organization.Id).Updates(map[string]interface{}{
			"allow_member_topup": params.AllowMemberTopup,
			"policy_version":     nextPolicyVersion,
		}).Error; err != nil {
			return err
		}
		organization.AllowMemberTopup = params.AllowMemberTopup
		organization.PolicyVersion = nextPolicyVersion
		for _, userID := range userIDs {
			change, err := model.AdvanceOrganizationUserAuthorizationWithTx(tx, userID)
			if err != nil {
				return err
			}
			changes = append(changes, change)
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.topup_policy.update", "organization", strconv.Itoa(organization.Id), requestID, map[string]interface{}{
			"allow_member_topup": params.AllowMemberTopup,
			"policy_version":     nextPolicyVersion,
		})
	})
	if err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		if err := model.FinalizeOrganizationAuthorizationChanges(changes, "organization_topup_policy_changed"); err != nil {
			return &organization, err
		}
	}
	return &organization, nil
}

func ListOrganizationMembers(actorUserID, organizationID int, params ListOrganizationMembersParams) (*OrganizationMemberListResult, error) {
	return ListOrganizationMembersForPlatform(actorUserID, organizationID, params)
}

func UpdateOrganizationMemberTopupPolicyForPlatform(actorUserID int, params UpdateOrganizationTopupPolicyParams) (*model.Organization, error) {
	return UpdateOrganizationTopupPolicyForPlatform(actorUserID, params)
}
