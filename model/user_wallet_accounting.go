package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

var (
	// ErrOrganizationLedgerRequired prevents legacy quota helpers from
	// mutating an organization member's wallet without a ledger entry.
	ErrOrganizationLedgerRequired         = errors.New("organization wallet mutation requires the organization ledger")
	ErrOrganizationMemberTopupForbidden   = errors.New("organization members are not allowed to top up their wallets")
	errLegacyOrganizationTaskBillingStale = errors.New("legacy organization task billing state changed")
)

// UserWalletCreditParams describes a non-recoverable credit to the user's
// only wallet. Organization-funded allocations use AllocateOrganizationQuota
// instead, because those credits remain recoverable by the organization.
type UserWalletCreditParams struct {
	UserId         int
	Amount         int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type UserWalletCreditResult struct {
	OrganizationId int
	UserQuotaAfter int64
	AlreadyApplied bool
}

type UserWalletDebitParams struct {
	UserId         int
	Amount         int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type UserWalletDebitResult struct {
	OrganizationId int
	UserQuotaAfter int64
	AlreadyApplied bool
}

type UserWalletSetParams struct {
	UserId         int
	TargetQuota    int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type LegacyOrganizationTaskWalletParams struct {
	TaskId           int64
	UserId           int
	OrganizationId   int
	ExpectedQuota    int
	ExpectedRevision int64
	ActualQuota      int
	OperationId      string
}

type legacyOrganizationTaskWalletFundingResult struct {
	OrganizationId int
	WalletDelta    int64
	AlreadyApplied bool
	credit         UserWalletCreditResult
	debit          UserWalletDebitResult
}

func adjustLegacyOrganizationTaskWalletFundingTx(tx *gorm.DB, params LegacyOrganizationTaskWalletParams) (legacyOrganizationTaskWalletFundingResult, error) {
	if tx == nil || params.TaskId <= 0 || params.UserId <= 0 || params.OrganizationId <= 0 ||
		params.ExpectedQuota <= 0 || params.ExpectedQuota >= common.MaxQuota || params.ExpectedRevision < 0 ||
		params.ActualQuota < 0 || params.ActualQuota >= common.MaxQuota || params.ActualQuota == params.ExpectedQuota ||
		strings.TrimSpace(params.OperationId) == "" {
		return legacyOrganizationTaskWalletFundingResult{}, ErrOrganizationAccountingInvalid
	}

	delta := params.ActualQuota - params.ExpectedQuota
	actor := OrganizationAccountingActor{
		Kind:   OrganizationAccountingActorSystem,
		Policy: "legacy_async_task_billing",
	}
	result := legacyOrganizationTaskWalletFundingResult{
		OrganizationId: params.OrganizationId,
		WalletDelta:    -int64(delta),
	}
	if delta > 0 {
		debit, err := DebitUserWalletTx(tx, UserWalletDebitParams{
			UserId: params.UserId, Amount: int64(delta), SourceType: "legacy_async_task",
			SourceId: params.OperationId, IdempotencyKey: params.OperationId, RequestId: params.OperationId, Actor: actor,
		})
		if err != nil {
			return legacyOrganizationTaskWalletFundingResult{}, err
		}
		if debit.OrganizationId != params.OrganizationId {
			return legacyOrganizationTaskWalletFundingResult{}, ErrOrganizationIdentityInvalid
		}
		result.AlreadyApplied = debit.AlreadyApplied
		result.debit = debit
		return result, nil
	}

	credit, err := CreditUserWalletTx(tx, UserWalletCreditParams{
		UserId: params.UserId, Amount: int64(-delta), SourceType: "wallet_refund",
		SourceId: params.OperationId, IdempotencyKey: params.OperationId, RequestId: params.OperationId, Actor: actor,
	})
	if err != nil {
		return legacyOrganizationTaskWalletFundingResult{}, err
	}
	if credit.OrganizationId != params.OrganizationId {
		return legacyOrganizationTaskWalletFundingResult{}, ErrOrganizationIdentityInvalid
	}
	result.AlreadyApplied = credit.AlreadyApplied
	result.credit = credit
	return result, nil
}

// AdjustLegacyOrganizationTaskWallet atomically bridges one migrated task
// whose wallet charge predates organization reservations. The wallet ledger
// and task billing CAS commit together, so stale pollers with different actual
// quotas cannot both mutate the user's balance.
func AdjustLegacyOrganizationTaskWallet(params LegacyOrganizationTaskWalletParams) (bool, error) {
	if DB == nil {
		return false, ErrOrganizationAccountingInvalid
	}

	delta := params.ActualQuota - params.ExpectedQuota
	var funding legacyOrganizationTaskWalletFundingResult
	advanced := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		funding, err = adjustLegacyOrganizationTaskWalletFundingTx(tx, params)
		if err != nil {
			return err
		}

		task := Task{
			ID: params.TaskId, UserId: params.UserId, OrganizationId: params.OrganizationId,
			LegacyOrganizationWallet: true,
		}
		advanced, err = task.advanceBillingQuotaTx(tx, params.ExpectedQuota, params.ExpectedRevision, params.ActualQuota, true)
		if err != nil {
			return err
		}
		if !advanced && !funding.AlreadyApplied {
			return errLegacyOrganizationTaskBillingStale
		}
		return nil
	})
	if errors.Is(err, errLegacyOrganizationTaskBillingStale) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !advanced {
		return false, nil
	}
	if delta > 0 {
		syncUserWalletDebitCache(params.UserId, int64(delta), funding.debit, "legacy async task billing")
	} else {
		syncUserWalletCreditCache(params.UserId, int64(-delta), funding.credit, "legacy async task billing")
	}
	return true, nil
}

// CreditUserWallet routes a credit through the organization ledger when the
// user belongs to an organization. The direct update exists only for a legacy
// database that has not provisioned organizations yet.
func CreditUserWallet(params UserWalletCreditParams) (UserWalletCreditResult, error) {
	var result UserWalletCreditResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = CreditUserWalletTx(tx, params)
		return err
	})
	if err == nil {
		syncUserWalletCreditCache(params.UserId, params.Amount, result, "legacy wallet credit")
	}
	return result, err
}

func syncUserWalletCreditCache(userId int, amount int64, result UserWalletCreditResult, operation string) {
	if result.AlreadyApplied || amount <= 0 {
		return
	}
	if result.OrganizationId > 0 {
		syncOrganizationAccountingQuotaCache(userId, amount, OrganizationLedgerWalletCredit)
		return
	}
	syncCreditUserQuotaCache(userId, int(amount), operation)
}

// CreditUserWalletTx is the transaction-scoped form used by payment,
// redemption, check-in, and registration-adjacent system grants.
func CreditUserWalletTx(tx *gorm.DB, params UserWalletCreditParams) (UserWalletCreditResult, error) {
	if tx == nil || params.UserId <= 0 || params.Amount < 0 || params.Amount > int64(common.MaxQuota-1) ||
		strings.TrimSpace(params.SourceType) == "" || strings.TrimSpace(params.SourceId) == "" ||
		strings.TrimSpace(params.IdempotencyKey) == "" || strings.TrimSpace(params.RequestId) == "" {
		return UserWalletCreditResult{}, ErrOrganizationAccountingInvalid
	}
	if len(params.SourceType) > 32 || len(params.SourceId) > 128 || len(params.IdempotencyKey) > 128 || len(params.RequestId) > 64 {
		return UserWalletCreditResult{}, ErrOrganizationAccountingInvalid
	}

	var membership struct {
		OrganizationId int
	}
	if err := tx.Model(&User{}).Select("organization_id").Where("id = ?", params.UserId).First(&membership).Error; err != nil {
		return UserWalletCreditResult{}, err
	}
	if membership.OrganizationId > 0 {
		accounting, err := CreditOrganizationUserWalletTx(tx, OrganizationWalletCreditParams{
			OrganizationId: membership.OrganizationId,
			UserId:         params.UserId,
			Amount:         params.Amount,
			SourceType:     params.SourceType,
			SourceId:       params.SourceId,
			IdempotencyKey: params.IdempotencyKey,
			RequestId:      params.RequestId,
			Actor:          params.Actor,
		})
		if err != nil {
			return UserWalletCreditResult{}, err
		}
		return UserWalletCreditResult{
			OrganizationId: membership.OrganizationId,
			UserQuotaAfter: accounting.UserQuotaAfter,
			AlreadyApplied: accounting.AlreadyApplied,
		}, nil
	}
	provisioned, err := organizationProvisioned(tx)
	if err != nil {
		return UserWalletCreditResult{}, err
	}
	if provisioned {
		return UserWalletCreditResult{}, ErrOrganizationIdentityInvalid
	}
	var user User
	if err := lockForUpdate(tx).Where("id = ?", params.UserId).First(&user).Error; err != nil {
		return UserWalletCreditResult{}, err
	}
	if user.OrganizationId > 0 {
		return UserWalletCreditResult{}, ErrOrganizationIdentityInvalid
	}
	if params.Amount == 0 {
		return UserWalletCreditResult{UserQuotaAfter: int64(user.Quota)}, nil
	}
	maxCurrent := int64(common.MaxQuota-1) - params.Amount
	if int64(user.Quota) > maxCurrent {
		return UserWalletCreditResult{}, ErrOrganizationUserQuotaLimit
	}
	update := tx.Model(&User{}).
		Where("id = ? AND organization_id = 0 AND quota = ? AND quota <= ?", user.Id, user.Quota, maxCurrent).
		Update("quota", gorm.Expr("quota + ?", params.Amount))
	if update.Error != nil {
		return UserWalletCreditResult{}, update.Error
	}
	if update.RowsAffected != 1 {
		return UserWalletCreditResult{}, ErrOrganizationAccountingIdempotency
	}
	return UserWalletCreditResult{UserQuotaAfter: int64(user.Quota) + params.Amount}, nil
}

// DebitUserWallet charges the user's only wallet. Organization users are
// debited through the tenant ledger; legacy direct updates remain available
// only before the default organization has been provisioned.
func DebitUserWallet(params UserWalletDebitParams) (UserWalletDebitResult, error) {
	var result UserWalletDebitResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = DebitUserWalletTx(tx, params)
		return err
	})
	if err == nil {
		syncUserWalletDebitCache(params.UserId, params.Amount, result, "legacy wallet debit")
	}
	return result, err
}

func syncUserWalletDebitCache(userId int, amount int64, result UserWalletDebitResult, operation string) {
	if result.AlreadyApplied || amount <= 0 {
		return
	}
	if result.OrganizationId > 0 {
		syncOrganizationAccountingQuotaCache(userId, -amount, OrganizationLedgerWalletDebit)
		return
	}
	if err := cacheDecrUserQuota(userId, amount); err != nil {
		common.SysLog("failed to sync " + operation + " quota cache: " + err.Error())
	}
}

func DebitUserWalletTx(tx *gorm.DB, params UserWalletDebitParams) (UserWalletDebitResult, error) {
	if tx == nil || params.UserId <= 0 || params.Amount <= 0 || params.Amount > int64(common.MaxQuota-1) ||
		strings.TrimSpace(params.SourceType) == "" || strings.TrimSpace(params.SourceId) == "" ||
		strings.TrimSpace(params.IdempotencyKey) == "" || strings.TrimSpace(params.RequestId) == "" {
		return UserWalletDebitResult{}, ErrOrganizationAccountingInvalid
	}
	if len(params.SourceType) > 32 || len(params.SourceId) > 128 || len(params.IdempotencyKey) > 128 || len(params.RequestId) > 64 {
		return UserWalletDebitResult{}, ErrOrganizationAccountingInvalid
	}

	var membership struct {
		OrganizationId int
	}
	if err := tx.Model(&User{}).Select("organization_id").Where("id = ?", params.UserId).First(&membership).Error; err != nil {
		return UserWalletDebitResult{}, err
	}
	if membership.OrganizationId > 0 {
		accounting, err := DebitOrganizationUserWalletTx(tx, OrganizationWalletDebitParams{
			OrganizationId: membership.OrganizationId,
			UserId:         params.UserId,
			Amount:         params.Amount,
			SourceType:     params.SourceType,
			SourceId:       params.SourceId,
			IdempotencyKey: params.IdempotencyKey,
			RequestId:      params.RequestId,
			Actor:          params.Actor,
		})
		if err != nil {
			return UserWalletDebitResult{}, err
		}
		return UserWalletDebitResult{
			OrganizationId: membership.OrganizationId,
			UserQuotaAfter: accounting.UserQuotaAfter,
			AlreadyApplied: accounting.AlreadyApplied,
		}, nil
	}
	provisioned, err := organizationProvisioned(tx)
	if err != nil {
		return UserWalletDebitResult{}, err
	}
	if provisioned {
		return UserWalletDebitResult{}, ErrOrganizationIdentityInvalid
	}
	var user User
	if err := lockForUpdate(tx).Where("id = ?", params.UserId).First(&user).Error; err != nil {
		return UserWalletDebitResult{}, err
	}
	if user.OrganizationId > 0 {
		return UserWalletDebitResult{}, ErrOrganizationIdentityInvalid
	}
	if int64(user.Quota) < params.Amount {
		return UserWalletDebitResult{}, ErrOrganizationUserQuotaInsufficient
	}
	update := tx.Model(&User{}).
		Where("id = ? AND organization_id = 0 AND quota = ? AND quota >= ?", user.Id, user.Quota, params.Amount).
		Update("quota", gorm.Expr("quota - ?", params.Amount))
	if update.Error != nil {
		return UserWalletDebitResult{}, update.Error
	}
	if update.RowsAffected != 1 {
		return UserWalletDebitResult{}, ErrOrganizationUserQuotaInsufficient
	}
	return UserWalletDebitResult{UserQuotaAfter: int64(user.Quota) - params.Amount}, nil
}

// SetUserWalletQuota applies an absolute administrative wallet target without
// bypassing organization accounting. Repeating the same request is a no-op;
// replaying it after a different balance change produces an idempotency
// conflict rather than overwriting newer funds.
func SetUserWalletQuota(params UserWalletSetParams) error {
	if params.UserId <= 0 || params.TargetQuota < 0 || params.TargetQuota > int64(common.MaxQuota-1) {
		return ErrOrganizationAccountingInvalid
	}
	var creditResult UserWalletCreditResult
	var debitResult UserWalletDebitResult
	var delta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var snapshot struct {
			OrganizationId int
			Quota          int
		}
		if err := tx.Model(&User{}).Select("organization_id", "quota").Where("id = ?", params.UserId).First(&snapshot).Error; err != nil {
			return err
		}
		if snapshot.OrganizationId <= 0 {
			provisioned, err := organizationProvisioned(tx)
			if err != nil {
				return err
			}
			if provisioned {
				return ErrOrganizationIdentityInvalid
			}
		}
		delta = params.TargetQuota - int64(snapshot.Quota)
		if delta == 0 {
			if snapshot.OrganizationId > 0 {
				if err := LockOrganizationAccountingScopesTx(tx, []int{snapshot.OrganizationId}); err != nil {
					return err
				}
			}
			var current User
			if err := lockForUpdate(tx).Select("id", "organization_id", "quota").Where("id = ?", params.UserId).First(&current).Error; err != nil {
				return err
			}
			if current.OrganizationId != snapshot.OrganizationId {
				return ErrOrganizationIdentityInvalid
			}
			if int64(current.Quota) != params.TargetQuota {
				return ErrOrganizationAccountingIdempotency
			}
			return nil
		}
		if delta > 0 {
			var err error
			creditResult, err = CreditUserWalletTx(tx, UserWalletCreditParams{
				UserId:         params.UserId,
				Amount:         delta,
				SourceType:     params.SourceType,
				SourceId:       params.SourceId,
				IdempotencyKey: params.IdempotencyKey,
				RequestId:      params.RequestId,
				Actor:          params.Actor,
			})
			if err != nil {
				return err
			}
			if creditResult.OrganizationId != snapshot.OrganizationId || creditResult.UserQuotaAfter != params.TargetQuota {
				return ErrOrganizationAccountingIdempotency
			}
			return nil
		}
		var err error
		debitResult, err = DebitUserWalletTx(tx, UserWalletDebitParams{
			UserId:         params.UserId,
			Amount:         -delta,
			SourceType:     params.SourceType,
			SourceId:       params.SourceId,
			IdempotencyKey: params.IdempotencyKey,
			RequestId:      params.RequestId,
			Actor:          params.Actor,
		})
		if err != nil {
			return err
		}
		if debitResult.OrganizationId != snapshot.OrganizationId || debitResult.UserQuotaAfter != params.TargetQuota {
			return ErrOrganizationAccountingIdempotency
		}
		return nil
	})
	if err != nil {
		return err
	}
	if delta > 0 {
		syncUserWalletCreditCache(params.UserId, delta, creditResult, "wallet override")
	} else if delta < 0 {
		syncUserWalletDebitCache(params.UserId, -delta, debitResult, "wallet override")
	}
	return nil
}

// ValidateUserTopUpPermission is evaluated before a checkout is created. A
// later payment callback must still honor an already-paid order even if the
// organization changes its policy while the payment is in flight.
func ValidateUserTopUpPermission(userId int) error {
	return validateUserTopUpPermissionTx(DB, userId)
}

func validateUserTopUpPermissionTx(tx *gorm.DB, userId int) error {
	if userId <= 0 {
		return ErrOrganizationAccountingInvalid
	}
	var user User
	if err := tx.Select("id", "status", "organization_id", "organization_role", "organization_status").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.OrganizationId <= 0 {
		provisioned, err := organizationProvisioned(tx)
		if err != nil {
			return err
		}
		if provisioned {
			return ErrOrganizationIdentityInvalid
		}
		return nil
	}
	if user.Status != common.UserStatusEnabled || user.OrganizationStatus != OrganizationMemberStatusActive || !isOrganizationRole(user.OrganizationRole) {
		return ErrOrganizationMemberNotActive
	}
	var organization Organization
	if err := tx.Select("id", "status", "allow_member_topup").Where("id = ?", user.OrganizationId).First(&organization).Error; err != nil {
		return err
	}
	if organization.Status != OrganizationStatusActive {
		return ErrOrganizationNotActive
	}
	if user.OrganizationRole == OrganizationRoleMember && !organization.AllowMemberTopup {
		return ErrOrganizationMemberTopupForbidden
	}
	return nil
}

// requireLegacyUserQuotaMutation is used by old direct quota primitives. Once
// organizations are provisioned, those primitives must never touch an
// organization wallet or an invalid zero-organization user.
func requireLegacyUserQuotaMutation(userId int) error {
	if userId <= 0 {
		return gorm.ErrRecordNotFound
	}
	var snapshot struct {
		OrganizationId int
	}
	if err := DB.Model(&User{}).Select("organization_id").Where("id = ?", userId).First(&snapshot).Error; err != nil {
		return err
	}
	if snapshot.OrganizationId > 0 {
		return ErrOrganizationLedgerRequired
	}
	provisioned, err := organizationProvisioned(DB)
	if err != nil {
		return err
	}
	if provisioned {
		return ErrOrganizationIdentityInvalid
	}
	return nil
}
