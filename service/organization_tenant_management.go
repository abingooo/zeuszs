package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTenantOrganizationRequestInvalid = errors.New("invalid tenant organization request")
	ErrOrganizationMemberStatusInvalid  = errors.New("invalid organization member status")
	ErrOrganizationMemberLimitInvalid   = errors.New("invalid organization member consumption limit")
)

type TenantOrganizationSummary struct {
	OrganizationID   int                            `json:"organization_id"`
	Name             string                         `json:"name"`
	Status           model.OrganizationStatus       `json:"status"`
	CurrentUserRole  model.OrganizationRole         `json:"current_user_role"`
	MemberStatus     model.OrganizationMemberStatus `json:"member_status"`
	AllowMemberTopup bool                           `json:"allow_member_topup"`
	OwnerUserID      *int                           `json:"owner_user_id,omitempty"`
	PolicyVersion    *int64                         `json:"policy_version,omitempty"`
	FundQuota        *int64                         `json:"fund_quota,omitempty"`
	MemberCount      *int64                         `json:"member_count,omitempty"`
	CreatedAt        *int64                         `json:"created_at,omitempty"`
	UpdatedAt        *int64                         `json:"updated_at,omitempty"`
}

type CreateTenantOrganizationInviteParams struct {
	Code      string
	MaxUses   int
	ExpiresAt int64
	RequestID string
}

type DisableTenantOrganizationInviteParams struct {
	InviteID  int
	RequestID string
}

type UpdateTenantOrganizationTopupPolicyParams struct {
	AllowMemberTopup bool
	RequestID        string
}

type UpdateTenantOrganizationMemberStatusParams struct {
	UserID    int
	Status    model.OrganizationMemberStatus
	RequestID string
}

type UpdateTenantOrganizationMemberLimitParams struct {
	UserID           int
	ConsumptionLimit *int64
	RequestID        string
}

type DisableTenantOrganizationMemberTokensParams struct {
	UserID    int
	RequestID string
}

type TransferTenantOrganizationQuotaParams struct {
	UserID         int
	Amount         int64
	IdempotencyKey string
	RequestID      string
}

type TenantOrganizationQuotaTransferResult struct {
	LedgerID              int64 `json:"ledger_id"`
	UserQuotaAfter        int64 `json:"user_quota_after"`
	PoolQuotaAfter        int64 `json:"pool_quota_after"`
	RecoverableQuotaAfter int64 `json:"recoverable_quota_after"`
	AlreadyApplied        bool  `json:"already_applied"`
}

type ListTenantOrganizationLedgerParams struct {
	Offset    int
	Limit     int
	UserID    int
	Operation string
}

type TenantOrganizationLedgerView struct {
	ID                    int64  `json:"id"`
	UserID                int    `json:"user_id"`
	ProjectID             *int   `json:"project_id,omitempty"`
	Operation             string `json:"operation"`
	SourceType            string `json:"source_type"`
	SourceID              string `json:"source_id"`
	ActorUserID           int    `json:"actor_user_id"`
	RequestID             string `json:"request_id"`
	UserQuotaDelta        int64  `json:"user_quota_delta"`
	PoolQuotaDelta        int64  `json:"pool_quota_delta"`
	RecoverableQuotaDelta int64  `json:"recoverable_quota_delta"`
	UserQuotaAfter        int64  `json:"user_quota_after"`
	PoolQuotaAfter        int64  `json:"pool_quota_after"`
	RecoverableQuotaAfter int64  `json:"recoverable_quota_after"`
	RelatedLedgerID       *int64 `json:"related_ledger_id,omitempty"`
	Status                string `json:"status"`
	CreatedAt             int64  `json:"created_at"`
}

type TenantOrganizationLedgerListResult struct {
	Items []TenantOrganizationLedgerView `json:"items"`
	Total int64                          `json:"total"`
}

type ListTenantOrganizationAuditParams struct {
	Offset      int
	Limit       int
	ActorUserID int
	Action      string
}

type TenantOrganizationAuditView struct {
	ID          int64           `json:"id"`
	ActorUserID int             `json:"actor_user_id"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	RequestID   string          `json:"request_id"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   int64           `json:"created_at"`
}

type TenantOrganizationAuditListResult struct {
	Items []TenantOrganizationAuditView `json:"items"`
	Total int64                         `json:"total"`
}

func requireTenantOrganizationActorTx(tx *gorm.DB, principal OrganizationPrincipal, action OrganizationAction) (*model.Organization, *model.User, error) {
	if tx == nil || principal.UserID <= 0 || principal.OrganizationID <= 0 {
		return nil, nil, ErrOrganizationIdentityInvalid
	}
	var organization model.Organization
	if err := model.LockForUpdate(tx).Where("id = ?", principal.OrganizationID).First(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrganizationIdentityInvalid
		}
		return nil, nil, err
	}
	if organization.Status != model.OrganizationStatusActive {
		return nil, nil, ErrOrganizationInactive
	}
	var actor model.User
	if err := model.LockForUpdate(tx).Where("id = ?", principal.UserID).First(&actor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrganizationIdentityInvalid
		}
		return nil, nil, err
	}
	if actor.Status != common.UserStatusEnabled ||
		actor.OrganizationId != organization.Id ||
		actor.OrganizationStatus != model.OrganizationMemberStatusActive ||
		actor.OrganizationRole != principal.Role {
		return nil, nil, ErrOrganizationIdentityInvalid
	}
	if actor.OrganizationRole == model.OrganizationRoleOwner && organization.OwnerUserId != actor.Id {
		return nil, nil, ErrOrganizationIdentityInvalid
	}
	current := OrganizationPrincipal{
		UserID:         actor.Id,
		OrganizationID: actor.OrganizationId,
		Role:           actor.OrganizationRole,
		PlatformRole:   actor.Role,
	}
	if !CanOrganizationAction(current, action) {
		return nil, nil, ErrOrganizationActionForbidden
	}
	return &organization, &actor, nil
}

func GetTenantOrganizationSummary(principal OrganizationPrincipal) (*TenantOrganizationSummary, error) {
	if model.DB == nil {
		return nil, ErrTenantOrganizationRequestInvalid
	}
	var summary TenantOrganizationSummary
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		organization, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionRead)
		if err != nil {
			return err
		}
		summary = TenantOrganizationSummary{
			OrganizationID:   organization.Id,
			Name:             organization.Name,
			Status:           organization.Status,
			CurrentUserRole:  actor.OrganizationRole,
			MemberStatus:     actor.OrganizationStatus,
			AllowMemberTopup: organization.AllowMemberTopup,
		}
		if actor.OrganizationRole == model.OrganizationRoleMember {
			return nil
		}
		var account model.OrganizationFundAccount
		if err := tx.Where("organization_id = ?", organization.Id).First(&account).Error; err != nil {
			return err
		}
		var memberCount int64
		if err := tx.Model(&model.User{}).
			Where("organization_id = ? AND organization_role = ?", organization.Id, model.OrganizationRoleMember).
			Count(&memberCount).Error; err != nil {
			return err
		}
		summary.OwnerUserID = &organization.OwnerUserId
		summary.PolicyVersion = &organization.PolicyVersion
		summary.FundQuota = &account.Quota
		summary.MemberCount = &memberCount
		summary.CreatedAt = &organization.CreatedAt
		summary.UpdatedAt = &organization.UpdatedAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func ListTenantOrganizationMembers(principal OrganizationPrincipal, params ListOrganizationMembersParams) (*OrganizationMemberListResult, error) {
	if err := validateOrganizationMemberListParams(&params); err != nil {
		return nil, err
	}
	if model.DB == nil {
		return nil, ErrOrganizationMemberQueryInvalid
	}
	result := &OrganizationMemberListResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		organization, _, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionMemberRead)
		if err != nil {
			return err
		}
		query := tx.Model(&model.User{}).
			Where("organization_id = ? AND organization_role = ?", organization.Id, model.OrganizationRoleMember)
		if params.Status != nil {
			query = query.Where("organization_status = ?", *params.Status)
		}
		if params.Keyword != "" {
			like := "%" + params.Keyword + "%"
			query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?", like, like, like)
		}
		if err := query.Count(&result.Total).Error; err != nil {
			return err
		}
		var users []model.User
		if err := query.Omit("password", "access_token").Order("id asc").Offset(params.Offset).Limit(params.Limit).Find(&users).Error; err != nil {
			return err
		}
		funds := make(map[int]model.OrganizationMemberFund, len(users))
		if len(users) > 0 {
			ids := make([]int, 0, len(users))
			for _, user := range users {
				ids = append(ids, user.Id)
			}
			var memberFunds []model.OrganizationMemberFund
			if err := tx.Where("organization_id = ? AND user_id IN ?", organization.Id, ids).Find(&memberFunds).Error; err != nil {
				return err
			}
			for _, fund := range memberFunds {
				funds[fund.UserId] = fund
			}
		}
		result.Items = make([]OrganizationMemberView, 0, len(users))
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
			result.Items = append(result.Items, view)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func CreateTenantOrganizationInvite(principal OrganizationPrincipal, params CreateTenantOrganizationInviteParams) (*OrganizationInviteView, error) {
	if err := validateOrganizationInviteMaxUses(params.MaxUses); err != nil {
		return nil, err
	}
	if err := validateOrganizationInviteExpiry(params.ExpiresAt); err != nil {
		return nil, err
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	if model.DB == nil {
		return nil, ErrOrganizationInviteManagementInvalid
	}
	customCode := strings.TrimSpace(params.Code) != ""
	for attempt := 0; attempt < 5; attempt++ {
		code, err := normalizeOrganizationInviteCodeForCreate(params.Code)
		if err != nil {
			return nil, err
		}
		var invite model.OrganizationInvite
		err = model.DB.Transaction(func(tx *gorm.DB) error {
			organization, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionInviteCreate)
			if err != nil {
				return err
			}
			invite = model.OrganizationInvite{
				OrganizationId: organization.Id,
				CodeHash:       HashOrganizationInviteCode(code),
				CodePrefix:     code[:organizationInvitePrefixLength(len(code))],
				Status:         model.OrganizationInviteStatusActive,
				MaxUses:        params.MaxUses,
				ExpiresAt:      params.ExpiresAt,
				DefaultRole:    model.OrganizationRoleMember,
				CreatedBy:      actor.Id,
			}
			if err := createOrganizationInviteRowTx(tx, &invite); err != nil {
				return err
			}
			return organizationAuditTx(tx, organization.Id, actor.Id, "organization.invite.create", "organization_invite", strconv.Itoa(invite.Id), requestID, map[string]interface{}{
				"code_prefix":  invite.CodePrefix,
				"max_uses":     invite.MaxUses,
				"expires_at":   invite.ExpiresAt,
				"default_role": string(model.OrganizationRoleMember),
			})
		})
		if errors.Is(err, ErrOrganizationInviteConflict) && !customCode {
			continue
		}
		if err != nil {
			return nil, err
		}
		view := organizationInviteView(invite)
		view.Code = code
		return &view, nil
	}
	return nil, ErrOrganizationInviteConflict
}

func ListTenantOrganizationInvites(principal OrganizationPrincipal, params ListOrganizationInvitesParams) (*OrganizationInviteListResult, error) {
	if params.Offset < 0 {
		params.Offset = 0
	}
	if params.Limit <= 0 {
		params.Limit = common.ItemsPerPage
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Status != nil {
		status, err := normalizeOrganizationInviteStatus(*params.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &status
	}
	if model.DB == nil {
		return nil, ErrOrganizationInviteManagementInvalid
	}
	result := &OrganizationInviteListResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		organization, _, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionInviteRead)
		if err != nil {
			return err
		}
		query := tx.Model(&model.OrganizationInvite{}).Where("organization_id = ?", organization.Id)
		if params.Status != nil {
			query = query.Where("status = ?", *params.Status)
		}
		if err := query.Count(&result.Total).Error; err != nil {
			return err
		}
		var invites []model.OrganizationInvite
		if err := query.Order("id desc").Offset(params.Offset).Limit(params.Limit).Find(&invites).Error; err != nil {
			return err
		}
		result.Items = make([]OrganizationInviteView, 0, len(invites))
		for _, invite := range invites {
			result.Items = append(result.Items, organizationInviteView(invite))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func DisableTenantOrganizationInvite(principal OrganizationPrincipal, params DisableTenantOrganizationInviteParams) (*OrganizationInviteView, error) {
	if params.InviteID <= 0 || model.DB == nil {
		return nil, ErrOrganizationInviteManagementInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	var invite model.OrganizationInvite
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		organization, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionInviteDisable)
		if err != nil {
			return err
		}
		if err := model.LockForUpdate(tx).Where("id = ? AND organization_id = ?", params.InviteID, organization.Id).First(&invite).Error; err != nil {
			return err
		}
		if invite.Status == model.OrganizationInviteStatusDisabled {
			return nil
		}
		result := tx.Model(&model.OrganizationInvite{}).
			Where("id = ? AND organization_id = ? AND status = ?", invite.Id, organization.Id, model.OrganizationInviteStatusActive).
			Update("status", model.OrganizationInviteStatusDisabled)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationInviteManagementInvalid
		}
		invite.Status = model.OrganizationInviteStatusDisabled
		return organizationAuditTx(tx, organization.Id, actor.Id, "organization.invite.disable", "organization_invite", strconv.Itoa(invite.Id), requestID, map[string]interface{}{
			"code_prefix": invite.CodePrefix,
		})
	})
	if err != nil {
		return nil, err
	}
	view := organizationInviteView(invite)
	return &view, nil
}

func UpdateTenantOrganizationTopupPolicy(principal OrganizationPrincipal, params UpdateTenantOrganizationTopupPolicyParams) (*model.Organization, error) {
	if model.DB == nil {
		return nil, ErrOrganizationPolicyInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	var organization model.Organization
	changes := make([]model.OrganizationAuthorizationChange, 0)
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionTopupPolicyUpdate)
		if err != nil {
			return err
		}
		organization = *current
		if organization.AllowMemberTopup == params.AllowMemberTopup {
			return nil
		}
		if organization.PolicyVersion == math.MaxInt64 {
			return ErrOrganizationPolicyInvalid
		}
		nextPolicyVersion := organization.PolicyVersion
		if nextPolicyVersion < 1 {
			nextPolicyVersion = 1
		}
		nextPolicyVersion++
		var userIDs []int
		if err := tx.Model(&model.User{}).Where("organization_id = ?", organization.Id).Order("id asc").Pluck("id", &userIDs).Error; err != nil {
			return err
		}
		result := tx.Model(&model.Organization{}).Where("id = ? AND policy_version = ?", organization.Id, organization.PolicyVersion).Updates(map[string]interface{}{
			"allow_member_topup": params.AllowMemberTopup,
			"policy_version":     nextPolicyVersion,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationPolicyInvalid
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
		return organizationAuditTx(tx, organization.Id, actor.Id, "organization.topup_policy.update", "organization", strconv.Itoa(organization.Id), requestID, map[string]interface{}{
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

func UpdateTenantOrganizationMemberStatus(principal OrganizationPrincipal, params UpdateTenantOrganizationMemberStatusParams) (*OrganizationMemberView, error) {
	status, err := normalizeOrganizationMemberStatus(params.Status)
	if err != nil || params.UserID <= 0 || model.DB == nil {
		return nil, ErrOrganizationMemberStatusInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	var target model.User
	var change *model.OrganizationAuthorizationChange
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		organization, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionMemberDisable)
		if err != nil {
			return err
		}
		if err := model.LockForUpdate(tx).Where("id = ?", params.UserID).First(&target).Error; err != nil {
			return err
		}
		current := OrganizationPrincipal{UserID: actor.Id, OrganizationID: actor.OrganizationId, Role: actor.OrganizationRole, PlatformRole: actor.Role}
		if err := AuthorizeOrganizationTarget(current, target.OrganizationId, target.OrganizationRole, OrganizationActionMemberDisable); err != nil {
			return err
		}
		if target.OrganizationStatus == status {
			return nil
		}
		oldStatus := target.OrganizationStatus
		result := tx.Model(&model.User{}).
			Where("id = ? AND organization_id = ? AND organization_role = ? AND organization_status = ?", target.Id, organization.Id, model.OrganizationRoleMember, oldStatus).
			Update("organization_status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationActionForbidden
		}
		target.OrganizationStatus = status
		version, err := model.AdvanceOrganizationUserAuthorizationWithTx(tx, target.Id)
		if err != nil {
			return err
		}
		change = &version
		target.AuthVersion = version.AuthVersion
		return organizationAuditTx(tx, organization.Id, actor.Id, "organization.member.status.update", "user", strconv.Itoa(target.Id), requestID, map[string]interface{}{
			"from": string(oldStatus),
			"to":   string(status),
		})
	})
	if err != nil {
		return nil, err
	}
	if change != nil {
		if err := model.FinalizeOrganizationAuthorizationChanges([]model.OrganizationAuthorizationChange{*change}, "organization_member_status_changed"); err != nil {
			view := tenantOrganizationMemberView(target, nil)
			return &view, err
		}
	}
	view := tenantOrganizationMemberView(target, nil)
	return &view, nil
}

func UpdateTenantOrganizationMemberLimit(principal OrganizationPrincipal, params UpdateTenantOrganizationMemberLimitParams) (*OrganizationMemberView, error) {
	if params.UserID <= 0 || model.DB == nil || (params.ConsumptionLimit != nil && *params.ConsumptionLimit < 0) {
		return nil, ErrOrganizationMemberLimitInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	var target model.User
	var memberFund model.OrganizationMemberFund
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		organization, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionMemberLimitUpdate)
		if err != nil {
			return err
		}
		if err := model.LockForUpdate(tx).Where("id = ?", params.UserID).First(&target).Error; err != nil {
			return err
		}
		current := OrganizationPrincipal{UserID: actor.Id, OrganizationID: actor.OrganizationId, Role: actor.OrganizationRole, PlatformRole: actor.Role}
		if err := AuthorizeOrganizationTarget(current, target.OrganizationId, target.OrganizationRole, OrganizationActionMemberLimitUpdate); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}},
			DoNothing: true,
		}).Create(&model.OrganizationMemberFund{OrganizationId: organization.Id, UserId: target.Id}).Error; err != nil {
			return err
		}
		if err := model.LockForUpdate(tx).Where("organization_id = ? AND user_id = ?", organization.Id, target.Id).First(&memberFund).Error; err != nil {
			return err
		}
		if (memberFund.ConsumptionLimit == nil && params.ConsumptionLimit == nil) ||
			(memberFund.ConsumptionLimit != nil && params.ConsumptionLimit != nil && *memberFund.ConsumptionLimit == *params.ConsumptionLimit) {
			return nil
		}
		oldLimit := memberFund.ConsumptionLimit
		var newLimit interface{}
		if params.ConsumptionLimit != nil {
			newLimit = *params.ConsumptionLimit
		}
		result := tx.Model(&model.OrganizationMemberFund{}).
			Where("id = ? AND organization_id = ? AND user_id = ?", memberFund.Id, organization.Id, target.Id).
			Update("consumption_limit", newLimit)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationMemberLimitInvalid
		}
		memberFund.ConsumptionLimit = params.ConsumptionLimit
		return organizationAuditTx(tx, organization.Id, actor.Id, "organization.member.limit.update", "user", strconv.Itoa(target.Id), requestID, map[string]interface{}{
			"from": oldLimit,
			"to":   params.ConsumptionLimit,
		})
	})
	if err != nil {
		return nil, err
	}
	view := tenantOrganizationMemberView(target, &memberFund)
	return &view, nil
}

func DisableTenantOrganizationMemberTokens(principal OrganizationPrincipal, params DisableTenantOrganizationMemberTokensParams) (int64, error) {
	if params.UserID <= 0 || model.DB == nil {
		return 0, ErrTenantOrganizationRequestInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return 0, err
	}
	var disabled int64
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		organization, actor, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionMemberTokenDisable)
		if err != nil {
			return err
		}
		var target model.User
		if err := model.LockForUpdate(tx).Where("id = ?", params.UserID).First(&target).Error; err != nil {
			return err
		}
		current := OrganizationPrincipal{UserID: actor.Id, OrganizationID: actor.OrganizationId, Role: actor.OrganizationRole, PlatformRole: actor.Role}
		if err := AuthorizeOrganizationTarget(current, target.OrganizationId, target.OrganizationRole, OrganizationActionMemberTokenDisable); err != nil {
			return err
		}
		disabled, err = model.DisableUserOrganizationTokensTx(tx, target.Id, organization.Id)
		if err != nil || disabled == 0 {
			return err
		}
		return organizationAuditTx(tx, organization.Id, actor.Id, "organization.member.tokens.disable", "user", strconv.Itoa(target.Id), requestID, map[string]interface{}{
			"disabled_token_count": disabled,
		})
	})
	return disabled, err
}

func AllocateTenantOrganizationQuota(principal OrganizationPrincipal, params TransferTenantOrganizationQuotaParams) (*TenantOrganizationQuotaTransferResult, error) {
	requestID, idempotencyKey, err := normalizeTenantOrganizationAccountingKeys(principal.OrganizationID, "allocate", params.UserID, params.RequestID, params.IdempotencyKey)
	if err != nil || params.Amount <= 0 || model.DB == nil {
		return nil, ErrTenantOrganizationRequestInvalid
	}
	if _, err := LoadOrganizationTarget(principal, params.UserID, OrganizationActionMemberAllocate); err != nil {
		return nil, err
	}
	result, err := model.AllocateOrganizationQuota(model.OrganizationQuotaTransferParams{
		OrganizationId: principal.OrganizationID,
		UserId:         params.UserID,
		Amount:         params.Amount,
		SourceType:     "tenant_allocation",
		SourceId:       strconv.Itoa(params.UserID),
		IdempotencyKey: idempotencyKey,
		RequestId:      requestID,
		Actor: model.OrganizationAccountingActor{
			Kind: model.OrganizationAccountingActorUser, UserId: principal.UserID,
			Policy: "tenant_organization_quota_management",
		},
	})
	if err != nil {
		return nil, err
	}
	view := tenantOrganizationQuotaTransferResult(result)
	return &view, nil
}

func RecoverTenantOrganizationQuota(principal OrganizationPrincipal, params TransferTenantOrganizationQuotaParams) (*TenantOrganizationQuotaTransferResult, error) {
	requestID, idempotencyKey, err := normalizeTenantOrganizationAccountingKeys(principal.OrganizationID, "recover", params.UserID, params.RequestID, params.IdempotencyKey)
	if err != nil || params.Amount <= 0 || model.DB == nil {
		return nil, ErrTenantOrganizationRequestInvalid
	}
	if _, err := LoadOrganizationTarget(principal, params.UserID, OrganizationActionMemberRecover); err != nil {
		return nil, err
	}
	result, err := model.RecoverOrganizationQuota(model.OrganizationQuotaTransferParams{
		OrganizationId: principal.OrganizationID,
		UserId:         params.UserID,
		Amount:         params.Amount,
		SourceType:     "tenant_recovery",
		SourceId:       strconv.Itoa(params.UserID),
		IdempotencyKey: idempotencyKey,
		RequestId:      requestID,
		Actor: model.OrganizationAccountingActor{
			Kind: model.OrganizationAccountingActorUser, UserId: principal.UserID,
			Policy: "tenant_organization_quota_management",
		},
	})
	if err != nil {
		return nil, err
	}
	view := tenantOrganizationQuotaTransferResult(result)
	return &view, nil
}

func normalizeTenantOrganizationAccountingKeys(organizationID int, operation string, userID int, requestID, idempotencyKey string) (string, string, error) {
	if organizationID <= 0 || userID <= 0 || (operation != "allocate" && operation != "recover") {
		return "", "", ErrTenantOrganizationRequestInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(requestID)
	if err != nil {
		return "", "", err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = requestID
	}
	if len(idempotencyKey) > 128 {
		return "", "", ErrTenantOrganizationRequestInvalid
	}
	digest := sha256.Sum256([]byte(idempotencyKey))
	namespacedKey := "tenant:" + strconv.Itoa(organizationID) + ":" + operation + ":" + strconv.Itoa(userID) + ":" + hex.EncodeToString(digest[:])
	return requestID, namespacedKey, nil
}

func tenantOrganizationQuotaTransferResult(result model.OrganizationAccountingResult) TenantOrganizationQuotaTransferResult {
	return TenantOrganizationQuotaTransferResult{
		LedgerID: result.LedgerId, UserQuotaAfter: result.UserQuotaAfter,
		PoolQuotaAfter: result.PoolQuotaAfter, RecoverableQuotaAfter: result.RecoverableQuotaAfter,
		AlreadyApplied: result.AlreadyApplied,
	}
}

func ListTenantOrganizationLedger(principal OrganizationPrincipal, params ListTenantOrganizationLedgerParams) (*TenantOrganizationLedgerListResult, error) {
	if params.Offset < 0 {
		params.Offset = 0
	}
	if params.Limit <= 0 {
		params.Limit = common.ItemsPerPage
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	params.Operation = strings.TrimSpace(params.Operation)
	if params.UserID < 0 || len(params.Operation) > 32 || model.DB == nil {
		return nil, ErrTenantOrganizationRequestInvalid
	}
	result := &TenantOrganizationLedgerListResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		organization, _, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionLedgerRead)
		if err != nil {
			return err
		}
		query := tx.Model(&model.OrganizationQuotaLedger{}).Where("organization_id = ?", organization.Id)
		if params.UserID > 0 {
			query = query.Where("user_id = ?", params.UserID)
		}
		if params.Operation != "" {
			query = query.Where("operation = ?", params.Operation)
		}
		if err := query.Count(&result.Total).Error; err != nil {
			return err
		}
		var ledgers []model.OrganizationQuotaLedger
		if err := query.Order("id desc").Offset(params.Offset).Limit(params.Limit).Find(&ledgers).Error; err != nil {
			return err
		}
		result.Items = make([]TenantOrganizationLedgerView, 0, len(ledgers))
		for _, ledger := range ledgers {
			result.Items = append(result.Items, TenantOrganizationLedgerView{
				ID: ledger.Id, UserID: ledger.UserId, ProjectID: ledger.ProjectId,
				Operation: ledger.Operation, SourceType: ledger.SourceType, SourceID: ledger.SourceId,
				ActorUserID: ledger.ActorUserId, RequestID: ledger.RequestId,
				UserQuotaDelta: ledger.UserQuotaDelta, PoolQuotaDelta: ledger.PoolQuotaDelta,
				RecoverableQuotaDelta: ledger.RecoverableQuotaDelta, UserQuotaAfter: ledger.UserQuotaAfter,
				PoolQuotaAfter: ledger.PoolQuotaAfter, RecoverableQuotaAfter: ledger.RecoverableQuotaAfter,
				RelatedLedgerID: ledger.RelatedLedgerId, Status: ledger.Status, CreatedAt: ledger.CreatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ListTenantOrganizationAudit(principal OrganizationPrincipal, params ListTenantOrganizationAuditParams) (*TenantOrganizationAuditListResult, error) {
	if params.Offset < 0 {
		params.Offset = 0
	}
	if params.Limit <= 0 {
		params.Limit = common.ItemsPerPage
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	params.Action = strings.TrimSpace(params.Action)
	if params.ActorUserID < 0 || len(params.Action) > 64 || model.DB == nil {
		return nil, ErrTenantOrganizationRequestInvalid
	}
	result := &TenantOrganizationAuditListResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		organization, _, err := requireTenantOrganizationActorTx(tx, principal, OrganizationActionAuditRead)
		if err != nil {
			return err
		}
		query := tx.Model(&model.OrganizationAuditEvent{}).Where("organization_id = ?", organization.Id)
		if params.ActorUserID > 0 {
			query = query.Where("actor_user_id = ?", params.ActorUserID)
		}
		if params.Action != "" {
			query = query.Where("action = ?", params.Action)
		}
		if err := query.Count(&result.Total).Error; err != nil {
			return err
		}
		var events []model.OrganizationAuditEvent
		if err := query.Order("id desc").Offset(params.Offset).Limit(params.Limit).Find(&events).Error; err != nil {
			return err
		}
		result.Items = make([]TenantOrganizationAuditView, 0, len(events))
		for _, event := range events {
			metadata := json.RawMessage(event.Metadata)
			var validated map[string]interface{}
			if err := common.Unmarshal(metadata, &validated); err != nil {
				return err
			}
			result.Items = append(result.Items, TenantOrganizationAuditView{
				ID: event.Id, ActorUserID: event.ActorUserId, Action: event.Action,
				TargetType: event.TargetType, TargetID: event.TargetId, RequestID: event.RequestId,
				Metadata: metadata, CreatedAt: event.CreatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func tenantOrganizationMemberView(user model.User, fund *model.OrganizationMemberFund) OrganizationMemberView {
	view := OrganizationMemberView{
		UserID: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
		PlatformRole: user.Role, OrganizationID: user.OrganizationId, OrganizationRole: user.OrganizationRole,
		OrganizationStatus: user.OrganizationStatus, Quota: user.Quota, UsedQuota: user.UsedQuota,
		RequestCount: user.RequestCount, CreatedAt: user.CreatedAt,
	}
	if fund != nil {
		view.RecoverableQuota = fund.RecoverableQuota
		view.ConsumedQuota = fund.ConsumedQuota
		view.ConsumptionLimit = fund.ConsumptionLimit
	}
	return view
}
