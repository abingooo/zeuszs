package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var (
	ErrOrganizationOwnerUsernameRequired = errors.New("organization owner username is required")
	ErrOrganizationOwnerPasswordInvalid  = errors.New("organization owner password is invalid")
	ErrOrganizationOwnerAccountInvalid   = errors.New("organization owner account is invalid")
	ErrOrganizationFundReferenceRequired = errors.New("organization fund credit reference is required")
)

// CreateOrganizationWithOwnerParams provisions a new tenant and a new common
// platform account as its Owner. Existing users are deliberately not accepted
// by this path because moving their wallet and resource history across tenant
// boundaries is a separate, higher-risk operation.
type CreateOrganizationWithOwnerParams struct {
	Name             string
	OwnerUsername    string
	OwnerPassword    string
	OwnerDisplayName string
	OwnerEmail       string
	AllowMemberTopup *bool
	RequestID        string
}

type ProvisionedOrganizationOwner struct {
	UserID             int                            `json:"user_id"`
	Username           string                         `json:"username"`
	DisplayName        string                         `json:"display_name"`
	Email              string                         `json:"email,omitempty"`
	OrganizationID     int                            `json:"organization_id"`
	OrganizationRole   model.OrganizationRole         `json:"organization_role"`
	OrganizationStatus model.OrganizationMemberStatus `json:"organization_status"`
}

type ProvisionedOrganization struct {
	Organization model.Organization           `json:"organization"`
	Owner        ProvisionedOrganizationOwner `json:"owner"`
}

type CreditOrganizationFundForPlatformParams struct {
	OrganizationID int
	Amount         int64
	Reference      string
	RequestID      string
}

// CreateOrganizationWithOwnerForPlatform is the normal organization creation
// path after the single-organization invariant is enabled. The owner account,
// tenant rows, and initial wallet grant are committed together, so no visible
// user can exist without exactly one organization.
func CreateOrganizationWithOwnerForPlatform(actorUserID int, params CreateOrganizationWithOwnerParams) (*ProvisionedOrganization, error) {
	name, err := normalizeOrganizationName(params.Name)
	if err != nil {
		return nil, err
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	username := strings.TrimSpace(params.OwnerUsername)
	if username == "" {
		return nil, ErrOrganizationOwnerUsernameRequired
	}
	if len(params.OwnerPassword) < 8 || len(params.OwnerPassword) > 20 {
		return nil, ErrOrganizationOwnerPasswordInvalid
	}
	displayName := strings.TrimSpace(params.OwnerDisplayName)
	if displayName == "" {
		displayName = username
	}
	owner := model.User{
		Username:    username,
		Password:    params.OwnerPassword,
		DisplayName: displayName,
		Email:       model.NormalizeEmail(params.OwnerEmail),
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	if err := common.Validate.Struct(&owner); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOrganizationOwnerAccountInvalid, err)
	}
	initialQuota := int64(common.QuotaForNewUser)
	if initialQuota < 0 || initialQuota > int64(common.MaxQuota-1) {
		return nil, model.ErrOrganizationUserQuotaLimit
	}
	allowMemberTopup := true
	if params.AllowMemberTopup != nil {
		allowMemberTopup = *params.AllowMemberTopup
	}
	if model.DB == nil {
		return nil, ErrOrganizationManagementInvalid
	}

	var organization model.Organization
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := requirePlatformOrganizationActorTx(tx, actorUserID); err != nil {
			return err
		}
		if err := owner.InsertWithTxWithoutInitialQuota(tx, 0); err != nil {
			return err
		}

		organization = model.Organization{
			Name:             name,
			Status:           model.OrganizationStatusActive,
			OwnerUserId:      owner.Id,
			AllowMemberTopup: allowMemberTopup,
			PolicyVersion:    1,
		}
		if err := tx.Create(&organization).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error; err != nil {
			return err
		}
		membership := tx.Model(&model.User{}).Where("id = ? AND organization_id = 0", owner.Id).Updates(map[string]interface{}{
			"organization_id":     organization.Id,
			"organization_role":   model.OrganizationRoleOwner,
			"organization_status": model.OrganizationMemberStatusActive,
		})
		if membership.Error != nil {
			return membership.Error
		}
		if membership.RowsAffected != 1 {
			return ErrOrganizationOwnerInvalid
		}
		if err := tx.Create(&model.OrganizationMemberFund{
			OrganizationId: organization.Id,
			UserId:         owner.Id,
		}).Error; err != nil {
			return err
		}
		if initialQuota > 0 {
			if _, err := model.CreditOrganizationUserWalletTx(tx, model.OrganizationWalletCreditParams{
				OrganizationId: organization.Id,
				UserId:         owner.Id,
				Amount:         initialQuota,
				SourceType:     "organization_owner_grant",
				SourceId:       strconv.Itoa(owner.Id),
				IdempotencyKey: fmt.Sprintf("organization:%d:owner:%d:initial", organization.Id, owner.Id),
				RequestId:      requestID,
				Actor: model.OrganizationAccountingActor{
					Kind:   model.OrganizationAccountingActorSystem,
					Policy: "platform_owner_provisioning",
				},
			}); err != nil {
				return err
			}
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.create", "organization", strconv.Itoa(organization.Id), requestID, map[string]interface{}{
			"name":               organization.Name,
			"owner_user_id":      owner.Id,
			"owner_username":     owner.Username,
			"allow_member_topup": organization.AllowMemberTopup,
		})
	})
	if err != nil {
		return nil, err
	}

	owner.OrganizationId = organization.Id
	owner.OrganizationRole = model.OrganizationRoleOwner
	owner.OrganizationStatus = model.OrganizationMemberStatusActive
	owner.Quota = int(initialQuota)
	owner.FinishInsert(0)
	return &ProvisionedOrganization{
		Organization: organization,
		Owner: ProvisionedOrganizationOwner{
			UserID:             owner.Id,
			Username:           owner.Username,
			DisplayName:        owner.DisplayName,
			Email:              owner.Email,
			OrganizationID:     organization.Id,
			OrganizationRole:   model.OrganizationRoleOwner,
			OrganizationStatus: model.OrganizationMemberStatusActive,
		},
	}, nil
}

// CreditOrganizationFundForPlatform records an operator-authorized external
// receipt or manual adjustment in the organization budget pool. It never
// credits a user wallet. The external reference is the durable idempotency
// identity, while RequestID only correlates one HTTP attempt with its audit.
func CreditOrganizationFundForPlatform(actorUserID int, params CreditOrganizationFundForPlatformParams) (model.OrganizationAccountingResult, error) {
	if params.OrganizationID <= 0 || params.Amount <= 0 || model.DB == nil {
		return model.OrganizationAccountingResult{}, model.ErrOrganizationAccountingInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return model.OrganizationAccountingResult{}, err
	}
	reference := strings.TrimSpace(params.Reference)
	if reference == "" {
		return model.OrganizationAccountingResult{}, ErrOrganizationFundReferenceRequired
	}
	if len(reference) > 128 {
		return model.OrganizationAccountingResult{}, model.ErrOrganizationAccountingInvalid
	}

	var result model.OrganizationAccountingResult
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := loadPlatformOrganizationActorTx(tx, actorUserID, false); err != nil {
			return err
		}
		var snapshot model.Organization
		if err := tx.Select("id", "status").Where("id = ?", params.OrganizationID).First(&snapshot).Error; err != nil {
			return err
		}
		if snapshot.Status != model.OrganizationStatusActive {
			return model.ErrOrganizationNotActive
		}
		var err error
		result, err = model.CreditOrganizationFundTx(tx, model.OrganizationFundCreditParams{
			OrganizationId: snapshot.Id,
			Amount:         params.Amount,
			SourceType:     "platform_fund_credit",
			SourceId:       reference,
			IdempotencyKey: fmt.Sprintf("platform-fund:%d:%x", snapshot.Id, common.Sha256Raw([]byte(reference))),
			RequestId:      requestID,
			Actor: model.OrganizationAccountingActor{
				Kind:   model.OrganizationAccountingActorUser,
				UserId: actorUserID,
				Policy: "platform_fund_credit",
			},
		})
		if err != nil {
			return err
		}
		organization, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, snapshot.Id)
		if err != nil {
			return err
		}
		if organization.Status != model.OrganizationStatusActive {
			return model.ErrOrganizationNotActive
		}
		return nil
	})
	return result, err
}
