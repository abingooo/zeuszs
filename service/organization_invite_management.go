package service

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrOrganizationInviteManagementInvalid = errors.New("invalid organization invite request")
	ErrOrganizationInviteConflict          = errors.New("organization invite code already exists")
	ErrOrganizationInviteMaxUsesInvalid    = errors.New("organization invite max uses is invalid")
	ErrOrganizationInviteExpiryInvalid     = errors.New("organization invite expiry is invalid")
)

const (
	organizationInviteGeneratedLength = 24
	organizationInviteMinLength       = 8
	organizationInviteMaxLength       = 64
	organizationInviteMaxUses         = 1000000000
)

// CreateOrganizationInviteParams contains platform-controlled invite policy.
// DefaultRole is intentionally absent: registration invites can only create
// ordinary organization members, never Owner/Admin accounts.
type CreateOrganizationInviteParams struct {
	OrganizationID int
	Code           string
	MaxUses        int
	ExpiresAt      int64
	RequestID      string
}

// OrganizationInviteView includes the plaintext code only for the create
// response. List and disable operations leave Code empty; the database stores
// only CodeHash and a short display prefix.
type OrganizationInviteView struct {
	ID             int                            `json:"id"`
	OrganizationID int                            `json:"organization_id"`
	CodePrefix     string                         `json:"code_prefix"`
	Status         model.OrganizationInviteStatus `json:"status"`
	MaxUses        int                            `json:"max_uses"`
	UsedCount      int                            `json:"used_count"`
	ExpiresAt      int64                          `json:"expires_at"`
	DefaultRole    model.OrganizationRole         `json:"default_role"`
	CreatedBy      int                            `json:"created_by"`
	CreatedAt      int64                          `json:"created_at"`
	UpdatedAt      int64                          `json:"updated_at"`
	Code           string                         `json:"code,omitempty"`
}

type ListOrganizationInvitesParams struct {
	Offset int
	Limit  int
	Status *model.OrganizationInviteStatus
}

type OrganizationInviteListResult struct {
	Items []OrganizationInviteView `json:"items"`
	Total int64                    `json:"total"`
}

type DisableOrganizationInviteParams struct {
	OrganizationID int
	InviteID       int
	RequestID      string
}

func normalizeOrganizationInviteStatus(status model.OrganizationInviteStatus) (model.OrganizationInviteStatus, error) {
	status = model.OrganizationInviteStatus(strings.ToLower(strings.TrimSpace(string(status))))
	switch status {
	case model.OrganizationInviteStatusActive, model.OrganizationInviteStatusDisabled:
		return status, nil
	default:
		return "", ErrOrganizationInviteManagementInvalid
	}
}

func validateOrganizationInviteMaxUses(maxUses int) error {
	if maxUses < 0 || maxUses > organizationInviteMaxUses {
		return ErrOrganizationInviteMaxUsesInvalid
	}
	return nil
}

func validateOrganizationInviteExpiry(expiresAt int64) error {
	if expiresAt != 0 && expiresAt <= common.GetTimestamp() {
		return ErrOrganizationInviteExpiryInvalid
	}
	return nil
}

func normalizeOrganizationInviteCodeForCreate(code string) (string, error) {
	code = NormalizeOrganizationInviteCode(code)
	if code == "" {
		generated, err := common.GenerateRandomCharsKey(organizationInviteGeneratedLength)
		if err != nil {
			return "", err
		}
		code = strings.ToUpper(generated)
	}
	if len(code) < organizationInviteMinLength || len(code) > organizationInviteMaxLength {
		return "", ErrOrganizationInviteManagementInvalid
	}
	// Restrict manually supplied codes to an ASCII alphabet so normalization
	// and copy/paste behavior are deterministic across registration clients.
	for i := 0; i < len(code); i++ {
		ch := code[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return "", ErrOrganizationInviteManagementInvalid
	}
	return code, nil
}

// CreateOrganizationInviteForPlatform creates a member-only invite. A
// database uniqueness conflict is returned as a stable application error;
// generated-code collisions are retried with a fresh random code.
func CreateOrganizationInviteForPlatform(actorUserID int, params CreateOrganizationInviteParams) (*OrganizationInviteView, error) {
	if params.OrganizationID <= 0 {
		return nil, ErrOrganizationInviteManagementInvalid
	}
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
		view, err := createOrganizationInviteAttempt(actorUserID, params, code, params.MaxUses, params.ExpiresAt, requestID)
		if errors.Is(err, ErrOrganizationInviteConflict) && !customCode {
			continue
		}
		if err != nil {
			return nil, err
		}
		return view, nil
	}
	return nil, ErrOrganizationInviteConflict
}

func createOrganizationInviteAttempt(actorUserID int, params CreateOrganizationInviteParams, code string, maxUses int, expiresAt int64, requestID string) (*OrganizationInviteView, error) {
	var invite model.OrganizationInvite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization := *current
		if organization.Status != model.OrganizationStatusActive {
			return model.ErrOrganizationNotActive
		}
		invite = model.OrganizationInvite{
			OrganizationId: params.OrganizationID,
			CodeHash:       HashOrganizationInviteCode(code),
			CodePrefix:     code[:organizationInvitePrefixLength(len(code))],
			Status:         model.OrganizationInviteStatusActive,
			MaxUses:        maxUses,
			ExpiresAt:      expiresAt,
			DefaultRole:    model.OrganizationRoleMember,
			CreatedBy:      actorUserID,
		}
		if err := createOrganizationInviteRowTx(tx, &invite); err != nil {
			return err
		}
		if err := organizationAuditTx(tx, organization.Id, actorUserID, "organization.invite.create", "organization_invite", strconv.Itoa(invite.Id), requestID, map[string]interface{}{
			"code_prefix":  code[:organizationInvitePrefixLength(len(code))],
			"max_uses":     maxUses,
			"expires_at":   expiresAt,
			"default_role": string(model.OrganizationRoleMember),
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	view := organizationInviteView(invite)
	view.Code = code
	return &view, nil
}

func createOrganizationInviteRowTx(tx *gorm.DB, invite *model.OrganizationInvite) error {
	if tx == nil || invite == nil || strings.TrimSpace(invite.CodeHash) == "" {
		return ErrOrganizationInviteManagementInvalid
	}
	invite.CreationToken = common.NewRequestId()
	attemptToken := invite.CreationToken
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code_hash"}},
		DoNothing: true,
	}).Create(invite).Error; err != nil {
		return err
	}

	var persisted model.OrganizationInvite
	if err := model.LockForUpdate(tx).Where("code_hash = ?", invite.CodeHash).First(&persisted).Error; err != nil {
		return err
	}
	if persisted.CreationToken != attemptToken {
		return ErrOrganizationInviteConflict
	}
	*invite = persisted
	return nil
}

func organizationInvitePrefixLength(length int) int {
	if length < 6 {
		return length
	}
	return 6
}

// ListOrganizationInvitesForPlatform lists invite metadata without exposing
// hashes or plaintext codes.
func ListOrganizationInvitesForPlatform(actorUserID, organizationID int, params ListOrganizationInvitesParams) (*OrganizationInviteListResult, error) {
	if actorUserID <= 0 || organizationID <= 0 || model.DB == nil {
		return nil, ErrPlatformProvisioningForbidden
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
	if params.Status != nil {
		status, err := normalizeOrganizationInviteStatus(*params.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &status
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := loadPlatformOrganizationActorTx(tx, actorUserID, false); err != nil {
			return err
		}
		var organization model.Organization
		return tx.Select("id").Where("id = ?", organizationID).First(&organization).Error
	}); err != nil {
		return nil, err
	}
	query := model.DB.Model(&model.OrganizationInvite{}).Where("organization_id = ?", organizationID)
	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var invites []model.OrganizationInvite
	if err := query.Order("id desc").Offset(params.Offset).Limit(params.Limit).Find(&invites).Error; err != nil {
		return nil, err
	}
	items := make([]OrganizationInviteView, 0, len(invites))
	for _, invite := range invites {
		items = append(items, organizationInviteView(invite))
	}
	return &OrganizationInviteListResult{Items: items, Total: total}, nil
}

// DisableOrganizationInviteForPlatform revokes an invite without deleting
// its audit/history row. Repeating the operation is idempotent.
func DisableOrganizationInviteForPlatform(actorUserID int, params DisableOrganizationInviteParams) (*OrganizationInviteView, error) {
	if params.OrganizationID <= 0 || params.InviteID <= 0 {
		return nil, ErrOrganizationInviteManagementInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	if model.DB == nil {
		return nil, ErrOrganizationInviteManagementInvalid
	}
	var invite model.OrganizationInvite
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization := *current
		if err := model.LockForUpdate(tx).Where("id = ? AND organization_id = ?", params.InviteID, organization.Id).First(&invite).Error; err != nil {
			return err
		}
		if invite.Status == model.OrganizationInviteStatusDisabled {
			return nil
		}
		if err := tx.Model(&model.OrganizationInvite{}).Where("id = ? AND status = ?", invite.Id, model.OrganizationInviteStatusActive).Update("status", model.OrganizationInviteStatusDisabled).Error; err != nil {
			return err
		}
		invite.Status = model.OrganizationInviteStatusDisabled
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.invite.disable", "organization_invite", strconv.Itoa(invite.Id), requestID, map[string]interface{}{
			"code_prefix": invite.CodePrefix,
		})
	})
	if err != nil {
		return nil, err
	}
	view := organizationInviteView(invite)
	return &view, nil
}

func organizationInviteView(invite model.OrganizationInvite) OrganizationInviteView {
	return OrganizationInviteView{
		ID:             invite.Id,
		OrganizationID: invite.OrganizationId,
		CodePrefix:     invite.CodePrefix,
		Status:         invite.Status,
		MaxUses:        invite.MaxUses,
		UsedCount:      invite.UsedCount,
		ExpiresAt:      invite.ExpiresAt,
		DefaultRole:    invite.DefaultRole,
		CreatedBy:      invite.CreatedBy,
		CreatedAt:      invite.CreatedAt,
		UpdatedAt:      invite.UpdatedAt,
	}
}

// Convenience aliases use the same names as the platform organization
// management handlers and make the service API discoverable to callers.
func CreateOrganizationInvite(actorUserID int, params CreateOrganizationInviteParams) (*OrganizationInviteView, error) {
	return CreateOrganizationInviteForPlatform(actorUserID, params)
}

func ListOrganizationInvites(actorUserID, organizationID int, params ListOrganizationInvitesParams) (*OrganizationInviteListResult, error) {
	return ListOrganizationInvitesForPlatform(actorUserID, organizationID, params)
}

func DisableOrganizationInvite(actorUserID int, params DisableOrganizationInviteParams) (*OrganizationInviteView, error) {
	return DisableOrganizationInviteForPlatform(actorUserID, params)
}
