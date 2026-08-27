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
	ErrOrganizationMemberUsernameRequired = errors.New("organization member username is required")
	ErrOrganizationMemberPasswordInvalid  = errors.New("organization member password is invalid")
	ErrOrganizationMemberAccountInvalid   = errors.New("organization member account is invalid")
	ErrOrganizationMemberRoleInvalid      = errors.New("organization member role must be admin or member")
)

type ProvisionOrganizationMemberParams struct {
	OrganizationID int
	Username       string
	Password       string
	DisplayName    string
	Email          string
	Role           model.OrganizationRole
	RequestID      string
}

// ProvisionOrganizationMemberForPlatform atomically creates a new account in
// an existing tenant. Only a platform Admin/Root may use this path, and Owner
// creation remains exclusive to organization creation or ownership transfer.
func ProvisionOrganizationMemberForPlatform(actorUserID int, params ProvisionOrganizationMemberParams) (*OrganizationMemberView, error) {
	if params.OrganizationID <= 0 || model.DB == nil {
		return nil, ErrOrganizationManagementInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	username := strings.TrimSpace(params.Username)
	if username == "" {
		return nil, ErrOrganizationMemberUsernameRequired
	}
	if len(params.Password) < 8 || len(params.Password) > 20 {
		return nil, ErrOrganizationMemberPasswordInvalid
	}
	role := model.OrganizationRole(strings.ToLower(strings.TrimSpace(string(params.Role))))
	if role != model.OrganizationRoleAdmin && role != model.OrganizationRoleMember {
		return nil, ErrOrganizationMemberRoleInvalid
	}
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = username
	}
	user := model.User{
		Username:           username,
		Password:           params.Password,
		DisplayName:        displayName,
		Email:              model.NormalizeEmail(params.Email),
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     params.OrganizationID,
		OrganizationRole:   role,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	if err := common.Validate.Struct(&user); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOrganizationMemberAccountInvalid, err)
	}
	initialQuota := int64(common.QuotaForNewUser)
	if initialQuota < 0 || initialQuota > int64(common.MaxQuota-1) {
		return nil, model.ErrOrganizationUserQuotaLimit
	}

	var accounting model.OrganizationAccountingResult
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization := *current
		if organization.Status != model.OrganizationStatusActive {
			return model.ErrOrganizationNotActive
		}
		if err := user.InsertWithTxWithoutInitialQuota(tx, 0); err != nil {
			return err
		}
		if err := tx.Create(&model.OrganizationMemberFund{
			OrganizationId: organization.Id,
			UserId:         user.Id,
		}).Error; err != nil {
			return err
		}
		accounting, err = model.CreditOrganizationUserWalletTx(tx, model.OrganizationWalletCreditParams{
			OrganizationId: organization.Id,
			UserId:         user.Id,
			Amount:         initialQuota,
			SourceType:     "platform_member_grant",
			SourceId:       strconv.Itoa(user.Id),
			IdempotencyKey: fmt.Sprintf("organization:%d:member:%d:initial", organization.Id, user.Id),
			RequestId:      requestID,
			Actor: model.OrganizationAccountingActor{
				Kind:   model.OrganizationAccountingActorUser,
				UserId: actorUserID,
				Policy: "platform_member_provisioning",
			},
		})
		if err != nil {
			return err
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.member.provision", "user", strconv.Itoa(user.Id), requestID, map[string]interface{}{
			"username":          user.Username,
			"organization_role": string(role),
		})
	})
	if err != nil {
		return nil, err
	}

	user.Quota = int(accounting.UserQuotaAfter)
	user.FinishInsert(0)
	return &OrganizationMemberView{
		UserID:             user.Id,
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		Email:              user.Email,
		PlatformRole:       user.Role,
		OrganizationID:     user.OrganizationId,
		OrganizationRole:   user.OrganizationRole,
		OrganizationStatus: user.OrganizationStatus,
		Quota:              user.Quota,
		CreatedAt:          user.CreatedAt,
	}, nil
}
