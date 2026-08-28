package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrOrganizationInviteInvalid   = errors.New("organization invite is invalid")
	ErrOrganizationInviteExpired   = errors.New("organization invite has expired")
	ErrOrganizationInviteExhausted = errors.New("organization invite has no remaining uses")
)

type UserRegistrationParams struct {
	User                   *model.User
	AffiliateCode          string
	OrganizationInviteCode string
	RequestID              string
	GenerateDefaultToken   bool
	AfterCreateTx          func(tx *gorm.DB, user *model.User) error
}

type UserRegistrationResult struct {
	User               model.User
	DefaultToken       *model.Token
	OrganizationInvite *model.OrganizationInviteUse
	InitialQuota       int64
	InviteeReward      int64
	InviterReward      int64
}

// ProvisionRootUserWithTx creates the first platform administrator together
// with the platform-owned default organization. Root setup is a special
// platform provisioning path, so it intentionally does not use
// RegisterUserWithTx (which always creates an ordinary organization member).
// Every persistent balance mutation is still written through the central
// organization ledger before the caller commits its transaction.
func ProvisionRootUserWithTx(tx *gorm.DB, user *model.User, initialQuota int64, requestID string) error {
	if tx == nil || user == nil || user.Role != common.RoleRootUser || user.Status != common.UserStatusEnabled {
		return errors.New("invalid root registration user")
	}
	if initialQuota < 0 || initialQuota > int64(common.MaxQuota-1) {
		return model.ErrOrganizationUserQuotaLimit
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	if len(requestID) > 64 {
		return errors.New("registration request id is too long")
	}

	// Insert with a zero wallet first. The default organization helper updates
	// the membership columns before the ledger operation locks the user.
	if err := user.InsertWithTxWithoutInitialQuota(tx, 0); err != nil {
		return err
	}
	organization, err := model.EnsureDefaultOrganizationForRootTx(tx, user.Id)
	if err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(&model.OrganizationMemberFund{
		OrganizationId: organization.Id,
		UserId:         user.Id,
	}).Error; err != nil {
		return err
	}
	if initialQuota > 0 {
		if _, err := model.CreditOrganizationUserWalletTx(tx, model.OrganizationWalletCreditParams{
			OrganizationId: organization.Id,
			UserId:         user.Id,
			Amount:         initialQuota,
			SourceType:     "setup_root_grant",
			SourceId:       strconv.Itoa(user.Id),
			IdempotencyKey: fmt.Sprintf("setup:%d:root-grant", user.Id),
			RequestId:      requestID,
			Actor: model.OrganizationAccountingActor{
				Kind:   model.OrganizationAccountingActorSystem,
				Policy: "setup_root_grant",
			},
		}); err != nil {
			return err
		}
	}

	// Keep the caller's object useful after the transaction, while the database
	// remains the source of truth for the membership and wallet state.
	user.OrganizationId = organization.Id
	user.OrganizationRole = model.OrganizationRoleOwner
	user.OrganizationStatus = model.OrganizationMemberStatusActive
	user.Quota = int(initialQuota)
	return nil
}

func NormalizeOrganizationInviteCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func HashOrganizationInviteCode(code string) string {
	digest := sha256.Sum256([]byte(NormalizeOrganizationInviteCode(code)))
	return hex.EncodeToString(digest[:])
}

// RegisterUser owns the registration transaction and finalizes only
// non-authoritative log/cache projections after it commits.
func RegisterUser(params UserRegistrationParams) (*UserRegistrationResult, error) {
	var result *UserRegistrationResult
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = RegisterUserWithTx(tx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	FinalizeRegisteredUser(result)
	return result, nil
}

// RegisterUserWithTx provisions every persistent part of a new account. It is
// also used inside one-time OAuth flow transactions, so callers must finalize
// projections only after their outer transaction commits.
func RegisterUserWithTx(tx *gorm.DB, params UserRegistrationParams) (*UserRegistrationResult, error) {
	if tx == nil || params.User == nil {
		return nil, errors.New("registration user is required")
	}
	requestID := strings.TrimSpace(params.RequestID)
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	if len(requestID) > 64 {
		return nil, errors.New("registration request id is too long")
	}

	organization, invite, err := resolveRegistrationOrganizationTx(tx, params.OrganizationInviteCode)
	if err != nil {
		return nil, err
	}
	inviter, err := resolveRegistrationInviterTx(tx, params.AffiliateCode)
	if err != nil {
		return nil, err
	}
	organizationIDs := []int{organization.Id}
	if inviter != nil {
		organizationIDs = append(organizationIDs, inviter.OrganizationId)
	}
	if err := model.LockOrganizationAccountingScopesTx(tx, organizationIDs); err != nil {
		return nil, err
	}
	if invite != nil {
		invite, err = lockRegistrationOrganizationInviteTx(tx, organization.Id, invite.Id)
		if err != nil {
			return nil, err
		}
	}

	user := params.User
	user.OrganizationId = organization.Id
	user.OrganizationRole = model.OrganizationRoleMember
	user.OrganizationStatus = model.OrganizationMemberStatusActive
	user.Quota = 0
	if inviter != nil {
		user.InviterId = inviter.Id
	} else {
		user.InviterId = 0
	}
	if err := user.InsertWithTxWithoutInitialQuota(tx, user.InviterId); err != nil {
		return nil, err
	}
	memberFund := model.OrganizationMemberFund{
		OrganizationId: user.OrganizationId,
		UserId:         user.Id,
	}
	if err := tx.Create(&memberFund).Error; err != nil {
		return nil, err
	}

	var inviteUse *model.OrganizationInviteUse
	if invite != nil {
		inviteUse, err = claimOrganizationInviteTx(tx, invite, user.Id, requestID)
		if err != nil {
			return nil, err
		}
	}
	if params.AfterCreateTx != nil {
		if err := params.AfterCreateTx(tx, user); err != nil {
			return nil, err
		}
	}

	var defaultToken *model.Token
	if params.GenerateDefaultToken {
		defaultToken, err = createDefaultTokenTx(tx, user)
		if err != nil {
			return nil, err
		}
	}

	systemActor := model.OrganizationAccountingActor{
		Kind:   model.OrganizationAccountingActorSystem,
		Policy: "registration",
	}
	initialQuota := int64(common.QuotaForNewUser)
	initialResult, err := model.CreditOrganizationUserWalletTx(tx, model.OrganizationWalletCreditParams{
		OrganizationId: user.OrganizationId,
		UserId:         user.Id,
		Amount:         initialQuota,
		SourceType:     "registration_grant",
		SourceId:       strconv.Itoa(user.Id),
		IdempotencyKey: fmt.Sprintf("registration:%d:initial", user.Id),
		RequestId:      requestID,
		Actor:          systemActor,
	})
	if err != nil {
		return nil, err
	}
	user.Quota = int(initialResult.UserQuotaAfter)

	var inviteeReward int64
	var inviterReward int64
	if inviter != nil && operation_setting.IsPaymentComplianceConfirmed() {
		inviteeReward = int64(common.QuotaForInvitee)
		if inviteeReward > 0 {
			credit, err := model.CreditOrganizationUserWalletTx(tx, model.OrganizationWalletCreditParams{
				OrganizationId: user.OrganizationId,
				UserId:         user.Id,
				Amount:         inviteeReward,
				SourceType:     "registration_referral_reward",
				SourceId:       strconv.Itoa(inviter.Id),
				IdempotencyKey: fmt.Sprintf("registration:%d:invitee-reward", user.Id),
				RequestId:      requestID,
				Actor:          systemActor,
			})
			if err != nil {
				return nil, err
			}
			user.Quota = int(credit.UserQuotaAfter)
		}

		inviterReward = int64(common.QuotaForInviter)
		if inviterReward > 0 {
			if _, err := model.CreditOrganizationUserWalletTx(tx, model.OrganizationWalletCreditParams{
				OrganizationId: inviter.OrganizationId,
				UserId:         inviter.Id,
				Amount:         inviterReward,
				SourceType:     "affiliate_reward",
				SourceId:       strconv.Itoa(user.Id),
				IdempotencyKey: fmt.Sprintf("registration:%d:inviter-reward", user.Id),
				RequestId:      requestID,
				Actor:          systemActor,
			}); err != nil {
				return nil, err
			}
		}
		updates := map[string]interface{}{
			"aff_count":   gorm.Expr("aff_count + ?", 1),
			"aff_history": gorm.Expr("aff_history + ?", inviterReward),
		}
		if err := tx.Model(&model.User{}).Where("id = ?", inviter.Id).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return &UserRegistrationResult{
		User:               *user,
		DefaultToken:       defaultToken,
		OrganizationInvite: inviteUse,
		InitialQuota:       initialQuota,
		InviteeReward:      inviteeReward,
		InviterReward:      inviterReward,
	}, nil
}

func resolveRegistrationOrganizationTx(tx *gorm.DB, inviteCode string) (*model.Organization, *model.OrganizationInvite, error) {
	normalizedCode := NormalizeOrganizationInviteCode(inviteCode)
	if normalizedCode == "" {
		var organization model.Organization
		if err := tx.Where("system_key = ?", model.DefaultOrganizationSystemKey).First(&organization).Error; err != nil {
			return nil, nil, err
		}
		if organization.Status != model.OrganizationStatusActive {
			return nil, nil, model.ErrOrganizationNotActive
		}
		return &organization, nil, nil
	}

	var invite model.OrganizationInvite
	if err := tx.Where("code_hash = ?", HashOrganizationInviteCode(normalizedCode)).First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrganizationInviteInvalid
		}
		return nil, nil, err
	}
	if err := validateRegistrationOrganizationInvite(&invite); err != nil {
		return nil, nil, err
	}
	var organization model.Organization
	if err := tx.Where("id = ?", invite.OrganizationId).First(&organization).Error; err != nil {
		return nil, nil, err
	}
	if organization.Status != model.OrganizationStatusActive {
		return nil, nil, model.ErrOrganizationNotActive
	}
	return &organization, &invite, nil
}

func lockRegistrationOrganizationInviteTx(tx *gorm.DB, organizationID, inviteID int) (*model.OrganizationInvite, error) {
	var invite model.OrganizationInvite
	if err := model.LockForUpdate(tx).
		Where("id = ? AND organization_id = ?", inviteID, organizationID).
		First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationInviteInvalid
		}
		return nil, err
	}
	if err := validateRegistrationOrganizationInvite(&invite); err != nil {
		return nil, err
	}
	return &invite, nil
}

func validateRegistrationOrganizationInvite(invite *model.OrganizationInvite) error {
	if invite == nil || invite.Id <= 0 || invite.Status != model.OrganizationInviteStatusActive || invite.DefaultRole != model.OrganizationRoleMember {
		return ErrOrganizationInviteInvalid
	}
	if invite.ExpiresAt > 0 && invite.ExpiresAt <= common.GetTimestamp() {
		return ErrOrganizationInviteExpired
	}
	if invite.MaxUses > 0 && invite.UsedCount >= invite.MaxUses {
		return ErrOrganizationInviteExhausted
	}
	return nil
}

func claimOrganizationInviteTx(tx *gorm.DB, invite *model.OrganizationInvite, userID int, requestID string) (*model.OrganizationInviteUse, error) {
	if invite == nil || invite.Id <= 0 || userID <= 0 {
		return nil, ErrOrganizationInviteInvalid
	}
	query := tx.Model(&model.OrganizationInvite{}).
		Where("id = ? AND organization_id = ? AND status = ?", invite.Id, invite.OrganizationId, model.OrganizationInviteStatusActive).
		Where("(max_uses = 0 OR used_count < max_uses)").
		Where("(expires_at = 0 OR expires_at > ?)", common.GetTimestamp())
	result := query.Update("used_count", gorm.Expr("used_count + 1"))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrOrganizationInviteExhausted
	}
	use := model.OrganizationInviteUse{
		OrganizationInviteId: invite.Id,
		OrganizationId:       invite.OrganizationId,
		UserId:               userID,
		RequestId:            requestID,
	}
	if err := tx.Create(&use).Error; err != nil {
		return nil, err
	}
	if err := organizationAuditTx(
		tx,
		invite.OrganizationId,
		userID,
		"organization.member.join",
		"user",
		strconv.Itoa(userID),
		requestID,
		map[string]interface{}{
			"organization_role": string(model.OrganizationRoleMember),
			"code_prefix":       invite.CodePrefix,
		},
	); err != nil {
		return nil, err
	}
	return &use, nil
}

func resolveRegistrationInviterTx(tx *gorm.DB, affiliateCode string) (*model.User, error) {
	affiliateCode = strings.TrimSpace(affiliateCode)
	if affiliateCode == "" {
		return nil, nil
	}
	var inviter model.User
	err := tx.Where("aff_code = ?", affiliateCode).First(&inviter).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if inviter.Status != common.UserStatusEnabled ||
		inviter.OrganizationId <= 0 ||
		inviter.OrganizationStatus != model.OrganizationMemberStatusActive {
		return nil, nil
	}
	var organization model.Organization
	if err := tx.Select("id", "status").Where("id = ?", inviter.OrganizationId).First(&organization).Error; err != nil {
		return nil, err
	}
	if organization.Status != model.OrganizationStatusActive {
		return nil, nil
	}
	return &inviter, nil
}

func createDefaultTokenTx(tx *gorm.DB, user *model.User) (*model.Token, error) {
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	token := model.Token{
		UserId:             user.Id,
		OrganizationId:     user.OrganizationId,
		Name:               user.Username + " initial key",
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        -1,
		RemainQuota:        500000,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
	}
	if setting.DefaultUseAutoGroup {
		token.Group = "auto"
	}
	if err := tx.Create(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func FinalizeRegisteredUser(result *UserRegistrationResult) {
	if result == nil || result.User.Id <= 0 {
		return
	}
	if result.InitialQuota > 0 {
		model.RecordLog(result.User.Id, model.LogTypeSystem, "Registration credit applied")
	}
	if result.InviteeReward > 0 {
		model.RecordLog(result.User.Id, model.LogTypeSystem, "Referral registration credit applied")
	}
	if result.InviterReward > 0 && result.User.InviterId > 0 {
		model.RecordLog(result.User.InviterId, model.LogTypeSystem, "Referral reward applied")
	}
	if result.DefaultToken != nil {
		_ = model.InvalidateUserTokensCache(result.User.Id)
	}
}
