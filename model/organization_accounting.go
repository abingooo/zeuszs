package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OrganizationAccountingActorUser   = "user"
	OrganizationAccountingActorSystem = "system"
)

const (
	OrganizationQuotaOperationPending   = "pending"
	OrganizationQuotaOperationCommitted = "committed"
)

const (
	OrganizationWalletReservationReserved = "reserved"
	OrganizationWalletReservationSettled  = "settled"
	OrganizationWalletReservationRefunded = "refunded"
)

const (
	OrganizationLedgerFundCredit   = "fund_credit"
	OrganizationLedgerWalletCredit = "wallet_credit"
	OrganizationLedgerWalletDebit  = "wallet_debit"
	OrganizationLedgerAllocate     = "allocation"
	OrganizationLedgerRecover      = "recovery"
	OrganizationLedgerReserve      = "wallet_reserve"
	OrganizationLedgerSettle       = "wallet_settle"
	OrganizationLedgerAdjust       = "wallet_adjust"
	OrganizationLedgerRefund       = "wallet_refund"
)

var (
	ErrOrganizationAccountingInvalid       = errors.New("invalid organization accounting request")
	ErrOrganizationAccountingForbidden     = errors.New("organization accounting operation forbidden")
	ErrOrganizationNotActive               = errors.New("organization is not active")
	ErrOrganizationMemberNotActive         = errors.New("organization member is not active")
	ErrOrganizationTargetNotMember         = errors.New("organization accounting target must be a member")
	ErrOrganizationFundInsufficient        = errors.New("organization fund quota insufficient")
	ErrOrganizationFundOverflow            = errors.New("organization fund quota overflow")
	ErrOrganizationUserQuotaInsufficient   = errors.New("user wallet quota insufficient")
	ErrOrganizationUserQuotaLimit          = errors.New("user wallet quota limit exceeded")
	ErrOrganizationRecoverableInsufficient = errors.New("recoverable organization quota insufficient")
	ErrOrganizationConsumptionLimit        = errors.New("organization member consumption limit exceeded")
	ErrOrganizationAccountingIdempotency   = errors.New("organization accounting idempotency conflict")
	ErrOrganizationAccountingPending       = errors.New("organization accounting operation is pending")
	ErrOrganizationReservationState        = errors.New("organization wallet reservation state conflict")
)

// OrganizationAccountingActor identifies the server-verified principal whose
// policy authorized an accounting mutation. System actors are reserved for
// trusted registration, payment-settlement, refund, and migration services.
type OrganizationAccountingActor struct {
	Kind   string
	UserId int
	Policy string
}

// OrganizationQuotaOperation claims an idempotency key before any balances
// are locked or changed. It is committed in the same transaction as its one
// ledger entry, so a pending row cannot survive a rollback.
type OrganizationQuotaOperation struct {
	Id                    int64  `json:"id"`
	IdempotencyKey        string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	ClaimToken            string `json:"-" gorm:"type:varchar(64)"`
	Fingerprint           string `json:"-" gorm:"type:char(64);not null"`
	Operation             string `json:"operation" gorm:"type:varchar(32);not null;index"`
	OrganizationId        int    `json:"organization_id" gorm:"not null;index"`
	UserId                int    `json:"user_id" gorm:"not null;index"`
	Amount                int64  `json:"amount" gorm:"type:bigint;not null"`
	SourceType            string `json:"source_type" gorm:"type:varchar(32);not null"`
	SourceId              string `json:"source_id" gorm:"type:varchar(128);not null"`
	ActorKind             string `json:"actor_kind" gorm:"type:varchar(16);not null"`
	ActorUserId           int    `json:"actor_user_id" gorm:"not null;index"`
	ActorPolicy           string `json:"actor_policy" gorm:"type:varchar(64);not null"`
	RequestId             string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	State                 string `json:"state" gorm:"type:varchar(16);not null;index"`
	LedgerId              int64  `json:"ledger_id" gorm:"type:bigint;not null"`
	UserQuotaAfter        int64  `json:"user_quota_after" gorm:"type:bigint;not null"`
	PoolQuotaAfter        int64  `json:"pool_quota_after" gorm:"type:bigint;not null"`
	RecoverableQuotaAfter int64  `json:"recoverable_quota_after" gorm:"type:bigint;not null"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// OrganizationWalletReservation preserves the exact organization-funded and
// self-funded parts of a wallet debit. Refunds restore that saved split rather
// than inferring a source from the member's later balance.
type OrganizationWalletReservation struct {
	Id                    int64  `json:"id"`
	OrganizationId        int    `json:"organization_id" gorm:"not null;index"`
	UserId                int    `json:"user_id" gorm:"not null;index"`
	RequestId             string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	ReserveIdempotencyKey string `json:"reserve_idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	ReservedQuota         int64  `json:"reserved_quota" gorm:"type:bigint;not null"`
	OrganizationQuota     int64  `json:"organization_quota" gorm:"type:bigint;not null"`
	SelfQuota             int64  `json:"self_quota" gorm:"type:bigint;not null"`
	SettledQuota          int64  `json:"settled_quota" gorm:"type:bigint;not null"`
	Status                string `json:"status" gorm:"type:varchar(16);not null;index"`
	ReserveLedgerId       int64  `json:"reserve_ledger_id" gorm:"type:bigint;not null"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type OrganizationFundCreditParams struct {
	OrganizationId int
	Amount         int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationWalletCreditParams struct {
	OrganizationId int
	UserId         int
	Amount         int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationWalletDebitParams struct {
	OrganizationId int
	UserId         int
	Amount         int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationQuotaTransferParams struct {
	OrganizationId int
	UserId         int
	Amount         int64
	SourceType     string
	SourceId       string
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationWalletReserveParams struct {
	OrganizationId int
	UserId         int
	Amount         int64
	RequestId      string
	IdempotencyKey string
	SourceType     string
	SourceId       string
	Actor          OrganizationAccountingActor
}

// OrganizationWalletReservationIncreaseParams describes an additional
// pre-consume against an existing, still-open wallet reservation. Tiered
// routing can discover a more expensive group after the initial reservation;
// the increment is recorded as its own idempotent ledger operation while the
// reservation remains in the reserved state.
type OrganizationWalletReservationIncreaseParams struct {
	ReservationId  int64
	Amount         int64
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationWalletSettleParams struct {
	ReservationId  int64
	ActualQuota    int64
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationWalletAdjustmentParams struct {
	ReservationId  int64
	ExpectedQuota  int64
	ActualQuota    int64
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationWalletRefundParams struct {
	ReservationId  int64
	IdempotencyKey string
	RequestId      string
	Actor          OrganizationAccountingActor
}

type OrganizationAccountingResult struct {
	LedgerId              int64
	UserQuotaAfter        int64
	PoolQuotaAfter        int64
	RecoverableQuotaAfter int64
	AlreadyApplied        bool
}

type OrganizationWalletReservationResult struct {
	Reservation OrganizationWalletReservation
	Accounting  OrganizationAccountingResult
}

func validateOrganizationAccountingIdentity(organizationId int, userId int, amount int64, sourceType string, sourceId string, idempotencyKey string, requestId string, actor OrganizationAccountingActor) error {
	if organizationId <= 0 || userId < 0 || amount < 0 || strings.TrimSpace(sourceType) == "" || strings.TrimSpace(sourceId) == "" || strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(requestId) == "" || strings.TrimSpace(actor.Policy) == "" {
		return ErrOrganizationAccountingInvalid
	}
	if len(idempotencyKey) > 128 || len(requestId) > 64 || len(sourceType) > 32 || len(sourceId) > 128 || len(actor.Policy) > 64 {
		return ErrOrganizationAccountingInvalid
	}
	switch actor.Kind {
	case OrganizationAccountingActorUser:
		if actor.UserId <= 0 {
			return ErrOrganizationAccountingInvalid
		}
	case OrganizationAccountingActorSystem:
		if actor.UserId != 0 {
			return ErrOrganizationAccountingInvalid
		}
	default:
		return ErrOrganizationAccountingInvalid
	}
	return nil
}

func organizationAccountingFingerprint(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func newOrganizationQuotaOperation(operation string, organizationId int, userId int, amount int64, sourceType string, sourceId string, idempotencyKey string, requestId string, actor OrganizationAccountingActor, extra ...string) *OrganizationQuotaOperation {
	parts := []string{
		operation,
		strconv.Itoa(organizationId),
		strconv.Itoa(userId),
		strconv.FormatInt(amount, 10),
		sourceType,
		sourceId,
		actor.Kind,
		strconv.Itoa(actor.UserId),
		actor.Policy,
	}
	parts = append(parts, extra...)
	return &OrganizationQuotaOperation{
		IdempotencyKey: idempotencyKey,
		ClaimToken:     common.NewRequestId(),
		Fingerprint:    organizationAccountingFingerprint(parts...),
		Operation:      operation,
		OrganizationId: organizationId,
		UserId:         userId,
		Amount:         amount,
		SourceType:     sourceType,
		SourceId:       sourceId,
		ActorKind:      actor.Kind,
		ActorUserId:    actor.UserId,
		ActorPolicy:    actor.Policy,
		RequestId:      requestId,
		State:          OrganizationQuotaOperationPending,
	}
}

func claimOrganizationQuotaOperation(tx *gorm.DB, operation *OrganizationQuotaOperation) (bool, error) {
	attemptClaimToken := operation.ClaimToken
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(operation).Error; err != nil {
		return false, err
	}

	var existing OrganizationQuotaOperation
	if err := lockForUpdate(tx).Where("idempotency_key = ?", operation.IdempotencyKey).First(&existing).Error; err != nil {
		return false, err
	}
	if existing.ClaimToken == attemptClaimToken {
		*operation = existing
		return false, nil
	}
	if existing.Fingerprint != operation.Fingerprint {
		return false, ErrOrganizationAccountingIdempotency
	}
	if existing.State != OrganizationQuotaOperationCommitted || existing.LedgerId <= 0 {
		return false, ErrOrganizationAccountingPending
	}
	*operation = existing
	return true, nil
}

func resultFromOrganizationQuotaOperation(operation *OrganizationQuotaOperation, alreadyApplied bool) OrganizationAccountingResult {
	return OrganizationAccountingResult{
		LedgerId:              operation.LedgerId,
		UserQuotaAfter:        operation.UserQuotaAfter,
		PoolQuotaAfter:        operation.PoolQuotaAfter,
		RecoverableQuotaAfter: operation.RecoverableQuotaAfter,
		AlreadyApplied:        alreadyApplied,
	}
}

func loadOrganizationFundAccount(tx *gorm.DB, organizationId int) (*Organization, *OrganizationFundAccount, error) {
	var organization Organization
	if err := lockForUpdate(tx).Where("id = ?", organizationId).First(&organization).Error; err != nil {
		return nil, nil, err
	}
	var account OrganizationFundAccount
	if err := lockForUpdate(tx).Where("organization_id = ?", organizationId).First(&account).Error; err != nil {
		return nil, nil, err
	}
	if account.Quota < 0 {
		return nil, nil, ErrOrganizationAccountingInvalid
	}
	return &organization, &account, nil
}

// LockOrganizationAccountingScopesTx acquires every organization and fund
// account in deterministic order before a trusted transaction performs
// accounting mutations across more than one organization.
func LockOrganizationAccountingScopesTx(tx *gorm.DB, organizationIds []int) error {
	if tx == nil || len(organizationIds) == 0 {
		return ErrOrganizationAccountingInvalid
	}
	unique := make(map[int]struct{}, len(organizationIds))
	ids := make([]int, 0, len(organizationIds))
	for _, organizationId := range organizationIds {
		if organizationId <= 0 {
			return ErrOrganizationAccountingInvalid
		}
		if _, exists := unique[organizationId]; exists {
			continue
		}
		unique[organizationId] = struct{}{}
		ids = append(ids, organizationId)
	}
	sort.Ints(ids)

	for _, organizationId := range ids {
		var organization Organization
		if err := lockForUpdate(tx).Where("id = ?", organizationId).First(&organization).Error; err != nil {
			return err
		}
		var account OrganizationFundAccount
		if err := lockForUpdate(tx).Where("organization_id = ?", organizationId).First(&account).Error; err != nil {
			return err
		}
		if organization.Id != organizationId || account.OrganizationId != organizationId || account.Quota < 0 {
			return ErrOrganizationAccountingInvalid
		}
	}
	return nil
}

func loadOrganizationMemberFund(tx *gorm.DB, organizationId int, userId int) (*OrganizationMemberFund, error) {
	initial := OrganizationMemberFund{OrganizationId: organizationId, UserId: userId}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(&initial).Error; err != nil {
		return nil, err
	}
	var fund OrganizationMemberFund
	if err := lockForUpdate(tx).Where("organization_id = ? AND user_id = ?", organizationId, userId).First(&fund).Error; err != nil {
		return nil, err
	}
	if fund.RecoverableQuota < 0 || fund.ConsumedQuota < 0 {
		return nil, ErrOrganizationAccountingInvalid
	}
	return &fund, nil
}

func validateOrganizationMemberFund(user *User, fund *OrganizationMemberFund) error {
	if user == nil || fund == nil || fund.OrganizationId != user.OrganizationId || fund.UserId != user.Id {
		return ErrOrganizationAccountingInvalid
	}
	if fund.RecoverableQuota < 0 || fund.RecoverableQuota > int64(user.Quota) {
		return ErrOrganizationAccountingInvalid
	}
	return nil
}

func authorizeOrganizationAccountingActor(tx *gorm.DB, organizationId int, actor OrganizationAccountingActor, organizationRoles ...OrganizationRole) error {
	if actor.Kind == OrganizationAccountingActorSystem {
		return nil
	}
	var actingUser User
	if err := lockForUpdate(tx).Where("id = ?", actor.UserId).First(&actingUser).Error; err != nil {
		return err
	}
	if actingUser.Status != common.UserStatusEnabled {
		return ErrOrganizationAccountingForbidden
	}
	if len(organizationRoles) == 0 {
		if actingUser.Role == common.RoleAdminUser || actingUser.Role == common.RoleRootUser {
			return nil
		}
		return ErrOrganizationAccountingForbidden
	}
	if actingUser.OrganizationId != organizationId || actingUser.OrganizationStatus != OrganizationMemberStatusActive {
		return ErrOrganizationAccountingForbidden
	}
	if actingUser.OrganizationRole == OrganizationRoleOwner {
		var organization Organization
		if err := lockForUpdate(tx).Select("id", "owner_user_id").Where("id = ?", organizationId).First(&organization).Error; err != nil {
			return err
		}
		if organization.OwnerUserId != actingUser.Id {
			return ErrOrganizationAccountingForbidden
		}
	}
	for _, role := range organizationRoles {
		if actingUser.OrganizationRole == role {
			return nil
		}
	}
	return ErrOrganizationAccountingForbidden
}

func loadOrganizationAccountingTarget(tx *gorm.DB, organizationId int, userId int, requireActive bool, requireMember bool) (*User, error) {
	var user User
	if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	if user.OrganizationId != organizationId {
		return nil, ErrOrganizationAccountingForbidden
	}
	if requireMember && user.OrganizationRole != OrganizationRoleMember {
		return nil, ErrOrganizationTargetNotMember
	}
	if requireActive && (user.Status != common.UserStatusEnabled || user.OrganizationStatus != OrganizationMemberStatusActive) {
		return nil, ErrOrganizationMemberNotActive
	}
	if user.Quota < 0 || int64(user.Quota) > int64(common.MaxQuota-1) {
		return nil, ErrOrganizationAccountingInvalid
	}
	return &user, nil
}

func commitOrganizationAccounting(tx *gorm.DB, operation *OrganizationQuotaOperation, ledger *OrganizationQuotaLedger, auditAction string, targetType string, targetId string) (OrganizationAccountingResult, error) {
	ledger.IdempotencyKey = operation.IdempotencyKey
	ledger.Fingerprint = operation.Fingerprint
	ledger.Status = OrganizationLedgerStatusCommitted
	if err := tx.Create(ledger).Error; err != nil {
		return OrganizationAccountingResult{}, err
	}

	metadata, err := common.Marshal(map[string]interface{}{
		"operation":               ledger.Operation,
		"source_type":             ledger.SourceType,
		"source_id":               ledger.SourceId,
		"actor_kind":              operation.ActorKind,
		"actor_policy":            operation.ActorPolicy,
		"idempotency_key":         operation.IdempotencyKey,
		"user_quota_delta":        ledger.UserQuotaDelta,
		"pool_quota_delta":        ledger.PoolQuotaDelta,
		"recoverable_quota_delta": ledger.RecoverableQuotaDelta,
		"user_quota_after":        ledger.UserQuotaAfter,
		"pool_quota_after":        ledger.PoolQuotaAfter,
		"recoverable_quota_after": ledger.RecoverableQuotaAfter,
	})
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	audit := OrganizationAuditEvent{
		OrganizationId: ledger.OrganizationId,
		ActorUserId:    ledger.ActorUserId,
		Action:         auditAction,
		TargetType:     targetType,
		TargetId:       targetId,
		RequestId:      ledger.RequestId,
		Metadata:       string(metadata),
	}
	if err := tx.Create(&audit).Error; err != nil {
		return OrganizationAccountingResult{}, err
	}

	updates := map[string]interface{}{
		"state":                   OrganizationQuotaOperationCommitted,
		"ledger_id":               ledger.Id,
		"user_quota_after":        ledger.UserQuotaAfter,
		"pool_quota_after":        ledger.PoolQuotaAfter,
		"recoverable_quota_after": ledger.RecoverableQuotaAfter,
	}
	result := tx.Model(&OrganizationQuotaOperation{}).
		Where("id = ? AND state = ?", operation.Id, OrganizationQuotaOperationPending).
		Updates(updates)
	if result.Error != nil {
		return OrganizationAccountingResult{}, result.Error
	}
	if result.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationAccountingIdempotency
	}
	operation.State = OrganizationQuotaOperationCommitted
	operation.LedgerId = ledger.Id
	operation.UserQuotaAfter = ledger.UserQuotaAfter
	operation.PoolQuotaAfter = ledger.PoolQuotaAfter
	operation.RecoverableQuotaAfter = ledger.RecoverableQuotaAfter
	return resultFromOrganizationQuotaOperation(operation, false), nil
}

func syncOrganizationAccountingQuotaCache(userId int, delta int64, operation string) {
	if userId <= 0 || delta == 0 || !common.RedisEnabled {
		return
	}
	result, err := cacheApplyUserQuotaDelta(userId, delta)
	if err == nil && result == cacheQuotaOK {
		return
	}
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to sync %s quota cache: %s", operation, err.Error()))
	}
	if invalidateErr := invalidateUserCache(userId); invalidateErr != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate user cache after %s: %s", operation, invalidateErr.Error()))
	}
}

func organizationQuotaLedger(operation *OrganizationQuotaOperation, userDelta int64, poolDelta int64, recoverableDelta int64, userAfter int64, poolAfter int64, recoverableAfter int64, relatedLedgerId *int64) *OrganizationQuotaLedger {
	return &OrganizationQuotaLedger{
		OrganizationId:        operation.OrganizationId,
		UserId:                operation.UserId,
		Operation:             operation.Operation,
		SourceType:            operation.SourceType,
		SourceId:              operation.SourceId,
		ActorUserId:           operation.ActorUserId,
		RequestId:             operation.RequestId,
		UserQuotaDelta:        userDelta,
		PoolQuotaDelta:        poolDelta,
		RecoverableQuotaDelta: recoverableDelta,
		UserQuotaAfter:        userAfter,
		PoolQuotaAfter:        poolAfter,
		RecoverableQuotaAfter: recoverableAfter,
		RelatedLedgerId:       relatedLedgerId,
	}
}

// CreditOrganizationFund credits the organization budget pool from an
// explicit, auditable source. It never changes a user's wallet.
func CreditOrganizationFund(params OrganizationFundCreditParams) (OrganizationAccountingResult, error) {
	var accounting OrganizationAccountingResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		accounting, err = CreditOrganizationFundTx(tx, params)
		return err
	})
	return accounting, err
}

// CreditOrganizationFundTx is a trusted transaction primitive. Callers that
// use a system actor must be an authenticated internal service.
func CreditOrganizationFundTx(tx *gorm.DB, params OrganizationFundCreditParams) (OrganizationAccountingResult, error) {
	if err := validateOrganizationAccountingIdentity(params.OrganizationId, 0, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor); err != nil || params.Amount <= 0 {
		if err != nil {
			return OrganizationAccountingResult{}, err
		}
		return OrganizationAccountingResult{}, ErrOrganizationAccountingInvalid
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerFundCredit, params.OrganizationId, 0, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if alreadyApplied {
		return resultFromOrganizationQuotaOperation(operation, true), nil
	}
	_, account, err := loadOrganizationFundAccount(tx, params.OrganizationId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := authorizeOrganizationAccountingActor(tx, params.OrganizationId, params.Actor); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if account.Quota > math.MaxInt64-params.Amount {
		return OrganizationAccountingResult{}, ErrOrganizationFundOverflow
	}
	result := tx.Model(&OrganizationFundAccount{}).
		Where("id = ? AND quota = ? AND quota <= ?", account.Id, account.Quota, math.MaxInt64-params.Amount).
		Update("quota", gorm.Expr("quota + ?", params.Amount))
	if result.Error != nil {
		return OrganizationAccountingResult{}, result.Error
	}
	if result.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationFundOverflow
	}
	poolAfter := account.Quota + params.Amount
	ledger := organizationQuotaLedger(operation, 0, params.Amount, 0, 0, poolAfter, 0, nil)
	return commitOrganizationAccounting(tx, operation, ledger, "organization.fund.credit", "organization", strconv.Itoa(params.OrganizationId))
}

// CreditOrganizationUserWallet credits the member's single wallet without
// increasing its recoverable organization-funded subset.
func CreditOrganizationUserWallet(params OrganizationWalletCreditParams) (OrganizationAccountingResult, error) {
	var accounting OrganizationAccountingResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		accounting, err = CreditOrganizationUserWalletTx(tx, params)
		return err
	})
	if err == nil && !accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(params.UserId, params.Amount, OrganizationLedgerWalletCredit)
	}
	return accounting, err
}

// CreditOrganizationUserWalletTx is used by registration and payment
// settlement after those services have verified their system actor and source.
func CreditOrganizationUserWalletTx(tx *gorm.DB, params OrganizationWalletCreditParams) (OrganizationAccountingResult, error) {
	if err := validateOrganizationAccountingIdentity(params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if params.Amount > int64(common.MaxQuota-1) {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaLimit
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerWalletCredit, params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if alreadyApplied {
		return resultFromOrganizationQuotaOperation(operation, true), nil
	}
	organization, account, err := loadOrganizationFundAccount(tx, params.OrganizationId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := authorizeOrganizationAccountingActor(tx, params.OrganizationId, params.Actor); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if organization.Status != OrganizationStatusActive && params.SourceType != "wallet_refund" && params.SourceType != "migration" {
		return OrganizationAccountingResult{}, ErrOrganizationNotActive
	}
	user, err := loadOrganizationAccountingTarget(tx, params.OrganizationId, params.UserId, params.SourceType != "wallet_refund" && params.SourceType != "migration", false)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, params.OrganizationId, params.UserId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if params.Amount == 0 {
		ledger := organizationQuotaLedger(operation, 0, 0, 0, int64(user.Quota), account.Quota, memberFund.RecoverableQuota, nil)
		return commitOrganizationAccounting(tx, operation, ledger, "organization.wallet.credit", "user", strconv.Itoa(params.UserId))
	}
	maxCurrent := int64(common.MaxQuota-1) - params.Amount
	if int64(user.Quota) > maxCurrent {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaLimit
	}
	result := tx.Model(&User{}).
		Where("id = ? AND organization_id = ? AND quota = ? AND quota <= ?", user.Id, params.OrganizationId, user.Quota, maxCurrent).
		Update("quota", gorm.Expr("quota + ?", params.Amount))
	if result.Error != nil {
		return OrganizationAccountingResult{}, result.Error
	}
	if result.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaLimit
	}
	userAfter := int64(user.Quota) + params.Amount
	ledger := organizationQuotaLedger(operation, params.Amount, 0, 0, userAfter, account.Quota, memberFund.RecoverableQuota, nil)
	return commitOrganizationAccounting(tx, operation, ledger, "organization.wallet.credit", "user", strconv.Itoa(params.UserId))
}

// DebitOrganizationUserWalletTx performs an immediate, non-refundable wallet
// consumption. Relay and asynchronous work use reservations instead; this
// primitive is for atomic purchases such as balance-paid subscriptions.
func DebitOrganizationUserWalletTx(tx *gorm.DB, params OrganizationWalletDebitParams) (OrganizationAccountingResult, error) {
	if err := validateOrganizationAccountingIdentity(params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor); err != nil || params.Amount <= 0 {
		if err != nil {
			return OrganizationAccountingResult{}, err
		}
		return OrganizationAccountingResult{}, ErrOrganizationAccountingInvalid
	}
	if params.Amount > int64(common.MaxQuota-1) {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaInsufficient
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerWalletDebit, params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if alreadyApplied {
		return resultFromOrganizationQuotaOperation(operation, true), nil
	}
	organization, account, err := loadOrganizationFundAccount(tx, params.OrganizationId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := authorizeOrganizationAccountingActor(tx, params.OrganizationId, params.Actor); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if organization.Status != OrganizationStatusActive {
		return OrganizationAccountingResult{}, ErrOrganizationNotActive
	}
	user, err := loadOrganizationAccountingTarget(tx, params.OrganizationId, params.UserId, true, false)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, params.OrganizationId, params.UserId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	debit, err := applyOrganizationWalletDebitTx(tx, user, memberFund, params.Amount)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	ledger := organizationQuotaLedger(operation, -params.Amount, 0, -debit.OrganizationQuota, debit.UserQuotaAfter, account.Quota, debit.RecoverableQuotaAfter, nil)
	return commitOrganizationAccounting(tx, operation, ledger, "organization.wallet.debit", "user", strconv.Itoa(params.UserId))
}

func AllocateOrganizationQuota(params OrganizationQuotaTransferParams) (OrganizationAccountingResult, error) {
	var accounting OrganizationAccountingResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		accounting, err = allocateOrganizationQuotaTx(tx, params)
		return err
	})
	if err == nil && !accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(params.UserId, params.Amount, OrganizationLedgerAllocate)
	}
	return accounting, err
}

func allocateOrganizationQuotaTx(tx *gorm.DB, params OrganizationQuotaTransferParams) (OrganizationAccountingResult, error) {
	if err := validateOrganizationAccountingIdentity(params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor); err != nil || params.Amount <= 0 {
		if err != nil {
			return OrganizationAccountingResult{}, err
		}
		return OrganizationAccountingResult{}, ErrOrganizationAccountingInvalid
	}
	if params.Amount > int64(common.MaxQuota-1) {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaLimit
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerAllocate, params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if alreadyApplied {
		return resultFromOrganizationQuotaOperation(operation, true), nil
	}
	organization, account, err := loadOrganizationFundAccount(tx, params.OrganizationId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := authorizeOrganizationAccountingActor(tx, params.OrganizationId, params.Actor, OrganizationRoleOwner, OrganizationRoleAdmin); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if organization.Status != OrganizationStatusActive {
		return OrganizationAccountingResult{}, ErrOrganizationNotActive
	}
	user, err := loadOrganizationAccountingTarget(tx, params.OrganizationId, params.UserId, true, true)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, params.OrganizationId, params.UserId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if account.Quota < params.Amount {
		return OrganizationAccountingResult{}, ErrOrganizationFundInsufficient
	}
	maxCurrent := int64(common.MaxQuota-1) - params.Amount
	if int64(user.Quota) > maxCurrent || memberFund.RecoverableQuota > math.MaxInt64-params.Amount {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaLimit
	}
	poolUpdate := tx.Model(&OrganizationFundAccount{}).
		Where("id = ? AND quota = ? AND quota >= ?", account.Id, account.Quota, params.Amount).
		Update("quota", gorm.Expr("quota - ?", params.Amount))
	if poolUpdate.Error != nil {
		return OrganizationAccountingResult{}, poolUpdate.Error
	}
	if poolUpdate.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationFundInsufficient
	}
	userUpdate := tx.Model(&User{}).
		Where("id = ? AND organization_id = ? AND quota = ? AND quota <= ?", user.Id, params.OrganizationId, user.Quota, maxCurrent).
		Update("quota", gorm.Expr("quota + ?", params.Amount))
	if userUpdate.Error != nil {
		return OrganizationAccountingResult{}, userUpdate.Error
	}
	if userUpdate.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaLimit
	}
	memberUpdate := tx.Model(&OrganizationMemberFund{}).
		Where("id = ? AND recoverable_quota = ? AND recoverable_quota <= ?", memberFund.Id, memberFund.RecoverableQuota, math.MaxInt64-params.Amount).
		Update("recoverable_quota", gorm.Expr("recoverable_quota + ?", params.Amount))
	if memberUpdate.Error != nil {
		return OrganizationAccountingResult{}, memberUpdate.Error
	}
	if memberUpdate.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationAccountingIdempotency
	}
	poolAfter := account.Quota - params.Amount
	userAfter := int64(user.Quota) + params.Amount
	recoverableAfter := memberFund.RecoverableQuota + params.Amount
	ledger := organizationQuotaLedger(operation, params.Amount, -params.Amount, params.Amount, userAfter, poolAfter, recoverableAfter, nil)
	return commitOrganizationAccounting(tx, operation, ledger, "organization.quota.allocate", "user", strconv.Itoa(params.UserId))
}

func RecoverOrganizationQuota(params OrganizationQuotaTransferParams) (OrganizationAccountingResult, error) {
	var accounting OrganizationAccountingResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		accounting, err = recoverOrganizationQuotaTx(tx, params)
		return err
	})
	if err == nil && !accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(params.UserId, -params.Amount, OrganizationLedgerRecover)
	}
	return accounting, err
}

func recoverOrganizationQuotaTx(tx *gorm.DB, params OrganizationQuotaTransferParams) (OrganizationAccountingResult, error) {
	if err := validateOrganizationAccountingIdentity(params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor); err != nil || params.Amount <= 0 {
		if err != nil {
			return OrganizationAccountingResult{}, err
		}
		return OrganizationAccountingResult{}, ErrOrganizationAccountingInvalid
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerRecover, params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if alreadyApplied {
		return resultFromOrganizationQuotaOperation(operation, true), nil
	}
	_, account, err := loadOrganizationFundAccount(tx, params.OrganizationId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := authorizeOrganizationAccountingActor(tx, params.OrganizationId, params.Actor, OrganizationRoleOwner, OrganizationRoleAdmin); err != nil {
		return OrganizationAccountingResult{}, err
	}
	user, err := loadOrganizationAccountingTarget(tx, params.OrganizationId, params.UserId, false, true)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, params.OrganizationId, params.UserId)
	if err != nil {
		return OrganizationAccountingResult{}, err
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return OrganizationAccountingResult{}, err
	}
	if int64(user.Quota) < params.Amount {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaInsufficient
	}
	if memberFund.RecoverableQuota < params.Amount {
		return OrganizationAccountingResult{}, ErrOrganizationRecoverableInsufficient
	}
	if account.Quota > math.MaxInt64-params.Amount {
		return OrganizationAccountingResult{}, ErrOrganizationFundOverflow
	}
	poolUpdate := tx.Model(&OrganizationFundAccount{}).
		Where("id = ? AND quota = ? AND quota <= ?", account.Id, account.Quota, math.MaxInt64-params.Amount).
		Update("quota", gorm.Expr("quota + ?", params.Amount))
	if poolUpdate.Error != nil {
		return OrganizationAccountingResult{}, poolUpdate.Error
	}
	if poolUpdate.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationFundOverflow
	}
	userUpdate := tx.Model(&User{}).
		Where("id = ? AND organization_id = ? AND quota = ? AND quota >= ?", user.Id, params.OrganizationId, user.Quota, params.Amount).
		Update("quota", gorm.Expr("quota - ?", params.Amount))
	if userUpdate.Error != nil {
		return OrganizationAccountingResult{}, userUpdate.Error
	}
	if userUpdate.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationUserQuotaInsufficient
	}
	memberUpdate := tx.Model(&OrganizationMemberFund{}).
		Where("id = ? AND recoverable_quota = ? AND recoverable_quota >= ?", memberFund.Id, memberFund.RecoverableQuota, params.Amount).
		Update("recoverable_quota", gorm.Expr("recoverable_quota - ?", params.Amount))
	if memberUpdate.Error != nil {
		return OrganizationAccountingResult{}, memberUpdate.Error
	}
	if memberUpdate.RowsAffected != 1 {
		return OrganizationAccountingResult{}, ErrOrganizationRecoverableInsufficient
	}
	poolAfter := account.Quota + params.Amount
	userAfter := int64(user.Quota) - params.Amount
	recoverableAfter := memberFund.RecoverableQuota - params.Amount
	ledger := organizationQuotaLedger(operation, -params.Amount, params.Amount, -params.Amount, userAfter, poolAfter, recoverableAfter, nil)
	return commitOrganizationAccounting(tx, operation, ledger, "organization.quota.recover", "user", strconv.Itoa(params.UserId))
}

type organizationWalletDebit struct {
	OrganizationQuota     int64
	SelfQuota             int64
	UserQuotaAfter        int64
	RecoverableQuotaAfter int64
	ConsumedQuotaAfter    int64
}

func applyOrganizationWalletDebitTx(tx *gorm.DB, user *User, memberFund *OrganizationMemberFund, amount int64) (organizationWalletDebit, error) {
	if amount <= 0 || amount > int64(common.MaxQuota-1) {
		return organizationWalletDebit{}, ErrOrganizationAccountingInvalid
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return organizationWalletDebit{}, err
	}
	if int64(user.Quota) < amount {
		return organizationWalletDebit{}, ErrOrganizationUserQuotaInsufficient
	}
	if memberFund.ConsumedQuota > math.MaxInt64-amount {
		return organizationWalletDebit{}, ErrOrganizationConsumptionLimit
	}
	consumedAfter := memberFund.ConsumedQuota + amount
	if memberFund.ConsumptionLimit != nil && consumedAfter > *memberFund.ConsumptionLimit {
		return organizationWalletDebit{}, ErrOrganizationConsumptionLimit
	}

	organizationQuota := amount
	if organizationQuota > memberFund.RecoverableQuota {
		organizationQuota = memberFund.RecoverableQuota
	}
	selfQuota := amount - organizationQuota
	userAfter := int64(user.Quota) - amount
	recoverableAfter := memberFund.RecoverableQuota - organizationQuota

	userUpdate := tx.Model(&User{}).
		Where("id = ? AND organization_id = ? AND quota = ? AND quota >= ?", user.Id, user.OrganizationId, user.Quota, amount).
		Update("quota", gorm.Expr("quota - ?", amount))
	if userUpdate.Error != nil {
		return organizationWalletDebit{}, userUpdate.Error
	}
	if userUpdate.RowsAffected != 1 {
		return organizationWalletDebit{}, ErrOrganizationUserQuotaInsufficient
	}

	memberQuery := tx.Model(&OrganizationMemberFund{}).
		Where("id = ? AND recoverable_quota = ? AND consumed_quota = ?", memberFund.Id, memberFund.RecoverableQuota, memberFund.ConsumedQuota).
		Where("consumption_limit IS NULL OR consumption_limit >= ?", consumedAfter)
	memberUpdate := memberQuery.Updates(map[string]interface{}{
		"recoverable_quota": recoverableAfter,
		"consumed_quota":    consumedAfter,
	})
	if memberUpdate.Error != nil {
		return organizationWalletDebit{}, memberUpdate.Error
	}
	if memberUpdate.RowsAffected != 1 {
		return organizationWalletDebit{}, ErrOrganizationConsumptionLimit
	}

	return organizationWalletDebit{
		OrganizationQuota:     organizationQuota,
		SelfQuota:             selfQuota,
		UserQuotaAfter:        userAfter,
		RecoverableQuotaAfter: recoverableAfter,
		ConsumedQuotaAfter:    consumedAfter,
	}, nil
}

type organizationWalletRefund struct {
	UserQuotaAfter        int64
	RecoverableQuotaAfter int64
	ConsumedQuotaAfter    int64
}

func applyOrganizationWalletRefundTx(tx *gorm.DB, user *User, memberFund *OrganizationMemberFund, organizationQuota int64, selfQuota int64) (organizationWalletRefund, error) {
	if organizationQuota < 0 || selfQuota < 0 || organizationQuota > math.MaxInt64-selfQuota {
		return organizationWalletRefund{}, ErrOrganizationAccountingInvalid
	}
	amount := organizationQuota + selfQuota
	if amount <= 0 || amount > int64(common.MaxQuota-1) {
		return organizationWalletRefund{}, ErrOrganizationAccountingInvalid
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return organizationWalletRefund{}, err
	}
	if int64(user.Quota) > int64(common.MaxQuota-1)-amount {
		return organizationWalletRefund{}, ErrOrganizationUserQuotaLimit
	}
	if memberFund.RecoverableQuota > math.MaxInt64-organizationQuota {
		return organizationWalletRefund{}, ErrOrganizationAccountingInvalid
	}
	userAfter := int64(user.Quota) + amount
	recoverableAfter := memberFund.RecoverableQuota + organizationQuota
	if memberFund.ConsumedQuota < amount {
		return organizationWalletRefund{}, ErrOrganizationAccountingInvalid
	}
	consumedAfter := memberFund.ConsumedQuota - amount
	if recoverableAfter > userAfter {
		return organizationWalletRefund{}, ErrOrganizationAccountingInvalid
	}

	userUpdate := tx.Model(&User{}).
		Where("id = ? AND organization_id = ? AND quota = ? AND quota <= ?", user.Id, user.OrganizationId, user.Quota, int64(common.MaxQuota-1)-amount).
		Update("quota", gorm.Expr("quota + ?", amount))
	if userUpdate.Error != nil {
		return organizationWalletRefund{}, userUpdate.Error
	}
	if userUpdate.RowsAffected != 1 {
		return organizationWalletRefund{}, ErrOrganizationUserQuotaLimit
	}
	memberUpdate := tx.Model(&OrganizationMemberFund{}).
		Where("id = ? AND recoverable_quota = ? AND consumed_quota = ?", memberFund.Id, memberFund.RecoverableQuota, memberFund.ConsumedQuota).
		Updates(map[string]interface{}{
			"recoverable_quota": recoverableAfter,
			"consumed_quota":    consumedAfter,
		})
	if memberUpdate.Error != nil {
		return organizationWalletRefund{}, memberUpdate.Error
	}
	if memberUpdate.RowsAffected != 1 {
		return organizationWalletRefund{}, ErrOrganizationAccountingIdempotency
	}
	return organizationWalletRefund{
		UserQuotaAfter:        userAfter,
		RecoverableQuotaAfter: recoverableAfter,
		ConsumedQuotaAfter:    consumedAfter,
	}, nil
}

func loadOrganizationWalletReservation(tx *gorm.DB, reservationId int64) (*OrganizationWalletReservation, error) {
	if reservationId <= 0 {
		return nil, ErrOrganizationAccountingInvalid
	}
	var reservation OrganizationWalletReservation
	if err := lockForUpdate(tx).Where("id = ?", reservationId).First(&reservation).Error; err != nil {
		return nil, err
	}
	if reservation.ReservedQuota < 0 || reservation.OrganizationQuota < 0 || reservation.SelfQuota < 0 || reservation.OrganizationQuota > math.MaxInt64-reservation.SelfQuota || reservation.OrganizationQuota+reservation.SelfQuota != reservation.ReservedQuota {
		return nil, ErrOrganizationAccountingInvalid
	}
	return &reservation, nil
}

// ReserveOrganizationWalletQuota atomically debits a member's only wallet,
// consumes organization-funded quota first, and saves the exact source split.
func ReserveOrganizationWalletQuota(params OrganizationWalletReserveParams) (OrganizationWalletReservationResult, error) {
	var reservationResult OrganizationWalletReservationResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		reservationResult, err = ReserveOrganizationWalletQuotaTx(tx, params)
		return err
	})
	if err == nil && !reservationResult.Accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(params.UserId, -params.Amount, OrganizationLedgerReserve)
	}
	return reservationResult, err
}

// ReserveOrganizationWalletQuotaTx is the durable wallet pre-consume
// primitive used by synchronous and asynchronous billing services.
func ReserveOrganizationWalletQuotaTx(tx *gorm.DB, params OrganizationWalletReserveParams) (OrganizationWalletReservationResult, error) {
	if err := validateOrganizationAccountingIdentity(params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor); err != nil || params.Amount <= 0 {
		if err != nil {
			return OrganizationWalletReservationResult{}, err
		}
		return OrganizationWalletReservationResult{}, ErrOrganizationAccountingInvalid
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerReserve, params.OrganizationId, params.UserId, params.Amount, params.SourceType, params.SourceId, params.IdempotencyKey, params.RequestId, params.Actor)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	if alreadyApplied {
		var reservation OrganizationWalletReservation
		if err := tx.Where("reserve_idempotency_key = ?", params.IdempotencyKey).First(&reservation).Error; err != nil {
			return OrganizationWalletReservationResult{}, err
		}
		return OrganizationWalletReservationResult{Reservation: reservation, Accounting: resultFromOrganizationQuotaOperation(operation, true)}, nil
	}
	organization, account, err := loadOrganizationFundAccount(tx, params.OrganizationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	if err := authorizeOrganizationAccountingActor(tx, params.OrganizationId, params.Actor); err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	if organization.Status != OrganizationStatusActive {
		return OrganizationWalletReservationResult{}, ErrOrganizationNotActive
	}
	user, err := loadOrganizationAccountingTarget(tx, params.OrganizationId, params.UserId, true, false)
	if err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, params.OrganizationId, params.UserId)
	if err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	debit, err := applyOrganizationWalletDebitTx(tx, user, memberFund, params.Amount)
	if err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	reservation := OrganizationWalletReservation{
		OrganizationId:        params.OrganizationId,
		UserId:                params.UserId,
		RequestId:             params.RequestId,
		ReserveIdempotencyKey: params.IdempotencyKey,
		ReservedQuota:         params.Amount,
		OrganizationQuota:     debit.OrganizationQuota,
		SelfQuota:             debit.SelfQuota,
		Status:                OrganizationWalletReservationReserved,
	}
	if err := tx.Create(&reservation).Error; err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	ledger := organizationQuotaLedger(operation, -params.Amount, 0, -debit.OrganizationQuota, debit.UserQuotaAfter, account.Quota, debit.RecoverableQuotaAfter, nil)
	accounting, err := commitOrganizationAccounting(tx, operation, ledger, "organization.wallet.reserve", "user", strconv.Itoa(params.UserId))
	if err != nil {
		return OrganizationWalletReservationResult{}, err
	}
	update := tx.Model(&OrganizationWalletReservation{}).
		Where("id = ? AND reserve_ledger_id = 0", reservation.Id).
		Update("reserve_ledger_id", accounting.LedgerId)
	if update.Error != nil {
		return OrganizationWalletReservationResult{}, update.Error
	}
	if update.RowsAffected != 1 {
		return OrganizationWalletReservationResult{}, ErrOrganizationAccountingIdempotency
	}
	reservation.ReserveLedgerId = accounting.LedgerId
	return OrganizationWalletReservationResult{Reservation: reservation, Accounting: accounting}, nil
}

// ReserveAdditionalOrganizationWalletQuota extends an open wallet
// reservation. It is used when routing/billing discovers a higher estimate
// after the first pre-consume. The extra debit preserves the same
// organization-funded/self-funded split and is independently idempotent.
func ReserveAdditionalOrganizationWalletQuota(params OrganizationWalletReservationIncreaseParams) (OrganizationWalletReservationResult, error) {
	var reservationResult OrganizationWalletReservationResult
	var userDelta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		reservationResult, userDelta, err = reserveAdditionalOrganizationWalletQuotaTx(tx, params)
		return err
	})
	if err == nil && !reservationResult.Accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(reservationResult.Reservation.UserId, userDelta, OrganizationLedgerReserve)
	}
	return reservationResult, err
}

func reserveAdditionalOrganizationWalletQuotaTx(tx *gorm.DB, params OrganizationWalletReservationIncreaseParams) (OrganizationWalletReservationResult, int64, error) {
	if tx == nil || params.ReservationId <= 0 || params.Amount <= 0 || params.Amount > int64(common.MaxQuota-1) ||
		strings.TrimSpace(params.IdempotencyKey) == "" || strings.TrimSpace(params.RequestId) == "" ||
		len(params.IdempotencyKey) > 128 || len(params.RequestId) > 64 || strings.TrimSpace(params.Actor.Policy) == "" {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationAccountingInvalid
	}

	// Read the reservation before claiming the operation so the operation
	// fingerprint is bound to its tenant and wallet owner. The row is locked
	// again below together with the balances before mutation.
	var snapshot OrganizationWalletReservation
	if err := tx.Where("id = ?", params.ReservationId).First(&snapshot).Error; err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := validateOrganizationAccountingIdentity(
		snapshot.OrganizationId,
		snapshot.UserId,
		params.Amount,
		"wallet_reservation",
		strconv.FormatInt(snapshot.Id, 10),
		params.IdempotencyKey,
		params.RequestId,
		params.Actor,
	); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}

	operation := newOrganizationQuotaOperation(
		OrganizationLedgerReserve,
		snapshot.OrganizationId,
		snapshot.UserId,
		params.Amount,
		"wallet_reservation",
		strconv.FormatInt(snapshot.Id, 10),
		params.IdempotencyKey,
		params.RequestId,
		params.Actor,
		strconv.FormatInt(params.ReservationId, 10),
	)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	reservation, err := loadOrganizationWalletReservation(tx, params.ReservationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if alreadyApplied {
		return OrganizationWalletReservationResult{
			Reservation: *reservation,
			Accounting:  resultFromOrganizationQuotaOperation(operation, true),
		}, 0, nil
	}
	if reservation.Status != OrganizationWalletReservationReserved {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
	}
	if reservation.ReservedQuota > int64(common.MaxQuota-1)-params.Amount {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationUserQuotaLimit
	}

	organization, account, err := loadOrganizationFundAccount(tx, reservation.OrganizationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := authorizeOrganizationAccountingActor(tx, reservation.OrganizationId, params.Actor); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if organization.Status != OrganizationStatusActive {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationNotActive
	}
	user, err := loadOrganizationAccountingTarget(tx, reservation.OrganizationId, reservation.UserId, true, false)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, reservation.OrganizationId, reservation.UserId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	debit, err := applyOrganizationWalletDebitTx(tx, user, memberFund, params.Amount)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}

	newReservedQuota := reservation.ReservedQuota + params.Amount
	newOrganizationQuota := reservation.OrganizationQuota + debit.OrganizationQuota
	newSelfQuota := reservation.SelfQuota + debit.SelfQuota
	if newOrganizationQuota < 0 || newSelfQuota < 0 || newOrganizationQuota > math.MaxInt64-newSelfQuota ||
		newOrganizationQuota+newSelfQuota != newReservedQuota {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationAccountingInvalid
	}
	reservationUpdate := tx.Model(&OrganizationWalletReservation{}).
		Where("id = ? AND status = ? AND reserved_quota = ? AND organization_quota = ? AND self_quota = ?",
			reservation.Id,
			OrganizationWalletReservationReserved,
			reservation.ReservedQuota,
			reservation.OrganizationQuota,
			reservation.SelfQuota,
		).
		Updates(map[string]interface{}{
			"reserved_quota":     newReservedQuota,
			"organization_quota": newOrganizationQuota,
			"self_quota":         newSelfQuota,
		})
	if reservationUpdate.Error != nil {
		return OrganizationWalletReservationResult{}, 0, reservationUpdate.Error
	}
	if reservationUpdate.RowsAffected != 1 {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
	}
	reservation.ReservedQuota = newReservedQuota
	reservation.OrganizationQuota = newOrganizationQuota
	reservation.SelfQuota = newSelfQuota
	ledger := organizationQuotaLedger(
		operation,
		-params.Amount,
		0,
		-debit.OrganizationQuota,
		debit.UserQuotaAfter,
		account.Quota,
		debit.RecoverableQuotaAfter,
		&reservation.ReserveLedgerId,
	)
	accounting, err := commitOrganizationAccounting(tx, operation, ledger, "organization.wallet.reserve", "wallet_reservation", strconv.FormatInt(reservation.Id, 10))
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	return OrganizationWalletReservationResult{Reservation: *reservation, Accounting: accounting}, params.Amount, nil
}

func SettleOrganizationWalletQuota(params OrganizationWalletSettleParams) (OrganizationWalletReservationResult, error) {
	var reservationResult OrganizationWalletReservationResult
	var userDelta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		reservationResult, userDelta, err = settleOrganizationWalletQuotaTx(tx, params)
		return err
	})
	if err == nil && !reservationResult.Accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(reservationResult.Reservation.UserId, userDelta, OrganizationLedgerSettle)
	}
	return reservationResult, err
}

func settleOrganizationWalletQuotaTx(tx *gorm.DB, params OrganizationWalletSettleParams) (OrganizationWalletReservationResult, int64, error) {
	return applyOrganizationWalletQuotaTargetTx(
		tx,
		params,
		nil,
		OrganizationWalletReservationReserved,
		OrganizationLedgerSettle,
		"organization.wallet.settle",
	)
}

// AdjustSettledOrganizationWalletQuota updates the final charge of a durable
// asynchronous reservation after its initial request has already settled.
func AdjustSettledOrganizationWalletQuota(params OrganizationWalletAdjustmentParams) (OrganizationWalletReservationResult, error) {
	if params.ExpectedQuota < 0 || params.ExpectedQuota > int64(common.MaxQuota-1) {
		return OrganizationWalletReservationResult{}, ErrOrganizationAccountingInvalid
	}
	var reservationResult OrganizationWalletReservationResult
	var userDelta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		reservationResult, userDelta, err = applyOrganizationWalletQuotaTargetTx(
			tx,
			OrganizationWalletSettleParams{
				ReservationId:  params.ReservationId,
				ActualQuota:    params.ActualQuota,
				IdempotencyKey: params.IdempotencyKey,
				RequestId:      params.RequestId,
				Actor:          params.Actor,
			},
			&params.ExpectedQuota,
			OrganizationWalletReservationSettled,
			OrganizationLedgerAdjust,
			"organization.wallet.adjust",
		)
		return err
	})
	if err == nil && !reservationResult.Accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(reservationResult.Reservation.UserId, userDelta, OrganizationLedgerAdjust)
	}
	return reservationResult, err
}

func applyOrganizationWalletQuotaTargetTx(tx *gorm.DB, params OrganizationWalletSettleParams, expectedQuota *int64, requiredStatus string, operationName string, auditAction string) (OrganizationWalletReservationResult, int64, error) {
	if params.ReservationId <= 0 || params.ActualQuota < 0 || params.ActualQuota > int64(common.MaxQuota-1) || strings.TrimSpace(params.IdempotencyKey) == "" || strings.TrimSpace(params.RequestId) == "" || len(params.IdempotencyKey) > 128 || len(params.RequestId) > 64 || strings.TrimSpace(params.Actor.Policy) == "" {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationAccountingInvalid
	}
	var snapshot OrganizationWalletReservation
	if err := tx.Where("id = ?", params.ReservationId).First(&snapshot).Error; err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := validateOrganizationAccountingIdentity(snapshot.OrganizationId, snapshot.UserId, params.ActualQuota, "wallet_reservation", strconv.FormatInt(snapshot.Id, 10), params.IdempotencyKey, params.RequestId, params.Actor); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	extraFingerprint := []string{strconv.FormatInt(params.ReservationId, 10)}
	if expectedQuota != nil {
		extraFingerprint = append(extraFingerprint, strconv.FormatInt(*expectedQuota, 10))
	}
	operation := newOrganizationQuotaOperation(operationName, snapshot.OrganizationId, snapshot.UserId, params.ActualQuota, "wallet_reservation", strconv.FormatInt(snapshot.Id, 10), params.IdempotencyKey, params.RequestId, params.Actor, extraFingerprint...)
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	reservation, err := loadOrganizationWalletReservation(tx, params.ReservationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if alreadyApplied {
		if expectedQuota != nil && reservation.SettledQuota != params.ActualQuota {
			return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
		}
		return OrganizationWalletReservationResult{Reservation: *reservation, Accounting: resultFromOrganizationQuotaOperation(operation, true)}, 0, nil
	}
	if reservation.Status != requiredStatus {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
	}
	if expectedQuota != nil && reservation.SettledQuota != *expectedQuota {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
	}
	_, account, err := loadOrganizationFundAccount(tx, reservation.OrganizationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := authorizeOrganizationAccountingActor(tx, reservation.OrganizationId, params.Actor); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	user, err := loadOrganizationAccountingTarget(tx, reservation.OrganizationId, reservation.UserId, false, false)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, reservation.OrganizationId, reservation.UserId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}

	userDelta := reservation.ReservedQuota - params.ActualQuota
	organizationDelta := int64(0)
	userAfter := int64(user.Quota)
	recoverableAfter := memberFund.RecoverableQuota
	newOrganizationQuota := reservation.OrganizationQuota
	newSelfQuota := reservation.SelfQuota
	if userDelta < 0 {
		debit, err := applyOrganizationWalletDebitTx(tx, user, memberFund, -userDelta)
		if err != nil {
			return OrganizationWalletReservationResult{}, 0, err
		}
		organizationDelta = -debit.OrganizationQuota
		userAfter = debit.UserQuotaAfter
		recoverableAfter = debit.RecoverableQuotaAfter
		newOrganizationQuota += debit.OrganizationQuota
		newSelfQuota += debit.SelfQuota
	} else if userDelta > 0 {
		selfRefund := userDelta
		if selfRefund > reservation.SelfQuota {
			selfRefund = reservation.SelfQuota
		}
		organizationRefund := userDelta - selfRefund
		if organizationRefund > reservation.OrganizationQuota {
			return OrganizationWalletReservationResult{}, 0, ErrOrganizationAccountingInvalid
		}
		refund, err := applyOrganizationWalletRefundTx(tx, user, memberFund, organizationRefund, selfRefund)
		if err != nil {
			return OrganizationWalletReservationResult{}, 0, err
		}
		organizationDelta = organizationRefund
		userAfter = refund.UserQuotaAfter
		recoverableAfter = refund.RecoverableQuotaAfter
		newOrganizationQuota -= organizationRefund
		newSelfQuota -= selfRefund
	}

	// A settled adjustment whose expected and actual quotas are equal has no
	// reservation fields to change. MySQL reports zero affected rows for that
	// no-op update by default, so commit its idempotency operation and ledger
	// without relying on dialect-specific RowsAffected behavior.
	if userDelta != 0 || reservation.Status != OrganizationWalletReservationSettled {
		reservationUpdate := tx.Model(&OrganizationWalletReservation{}).
			Where("id = ? AND status = ? AND reserved_quota = ? AND organization_quota = ? AND self_quota = ?", reservation.Id, requiredStatus, reservation.ReservedQuota, reservation.OrganizationQuota, reservation.SelfQuota).
			Updates(map[string]interface{}{
				"reserved_quota":     params.ActualQuota,
				"organization_quota": newOrganizationQuota,
				"self_quota":         newSelfQuota,
				"settled_quota":      params.ActualQuota,
				"status":             OrganizationWalletReservationSettled,
			})
		if reservationUpdate.Error != nil {
			return OrganizationWalletReservationResult{}, 0, reservationUpdate.Error
		}
		if reservationUpdate.RowsAffected != 1 {
			return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
		}
	}
	reservation.ReservedQuota = params.ActualQuota
	reservation.OrganizationQuota = newOrganizationQuota
	reservation.SelfQuota = newSelfQuota
	reservation.SettledQuota = params.ActualQuota
	reservation.Status = OrganizationWalletReservationSettled
	relatedLedgerId := reservation.ReserveLedgerId
	ledger := organizationQuotaLedger(operation, userDelta, 0, organizationDelta, userAfter, account.Quota, recoverableAfter, &relatedLedgerId)
	accounting, err := commitOrganizationAccounting(tx, operation, ledger, auditAction, "wallet_reservation", strconv.FormatInt(reservation.Id, 10))
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	return OrganizationWalletReservationResult{Reservation: *reservation, Accounting: accounting}, userDelta, nil
}

func RefundOrganizationWalletQuota(params OrganizationWalletRefundParams) (OrganizationWalletReservationResult, error) {
	var reservationResult OrganizationWalletReservationResult
	var userDelta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		reservationResult, userDelta, err = refundOrganizationWalletQuotaTx(tx, params)
		return err
	})
	if err == nil && !reservationResult.Accounting.AlreadyApplied {
		syncOrganizationAccountingQuotaCache(reservationResult.Reservation.UserId, userDelta, OrganizationLedgerRefund)
	}
	return reservationResult, err
}

func refundOrganizationWalletQuotaTx(tx *gorm.DB, params OrganizationWalletRefundParams) (OrganizationWalletReservationResult, int64, error) {
	if params.ReservationId <= 0 || strings.TrimSpace(params.IdempotencyKey) == "" || strings.TrimSpace(params.RequestId) == "" || len(params.IdempotencyKey) > 128 || len(params.RequestId) > 64 || strings.TrimSpace(params.Actor.Policy) == "" {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationAccountingInvalid
	}
	var snapshot OrganizationWalletReservation
	if err := tx.Where("id = ?", params.ReservationId).First(&snapshot).Error; err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := validateOrganizationAccountingIdentity(snapshot.OrganizationId, snapshot.UserId, snapshot.ReservedQuota, "wallet_reservation", strconv.FormatInt(snapshot.Id, 10), params.IdempotencyKey, params.RequestId, params.Actor); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	operation := newOrganizationQuotaOperation(OrganizationLedgerRefund, snapshot.OrganizationId, snapshot.UserId, snapshot.ReservedQuota, "wallet_reservation", strconv.FormatInt(snapshot.Id, 10), params.IdempotencyKey, params.RequestId, params.Actor, strconv.FormatInt(params.ReservationId, 10))
	alreadyApplied, err := claimOrganizationQuotaOperation(tx, operation)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	reservation, err := loadOrganizationWalletReservation(tx, params.ReservationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if alreadyApplied {
		return OrganizationWalletReservationResult{Reservation: *reservation, Accounting: resultFromOrganizationQuotaOperation(operation, true)}, 0, nil
	}
	if reservation.Status != OrganizationWalletReservationReserved && reservation.Status != OrganizationWalletReservationSettled {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
	}
	_, account, err := loadOrganizationFundAccount(tx, reservation.OrganizationId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := authorizeOrganizationAccountingActor(tx, reservation.OrganizationId, params.Actor); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	user, err := loadOrganizationAccountingTarget(tx, reservation.OrganizationId, reservation.UserId, false, false)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	memberFund, err := loadOrganizationMemberFund(tx, reservation.OrganizationId, reservation.UserId)
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	if err := validateOrganizationMemberFund(user, memberFund); err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	refund := organizationWalletRefund{
		UserQuotaAfter:        int64(user.Quota),
		RecoverableQuotaAfter: memberFund.RecoverableQuota,
		ConsumedQuotaAfter:    memberFund.ConsumedQuota,
	}
	if reservation.ReservedQuota > 0 {
		refund, err = applyOrganizationWalletRefundTx(tx, user, memberFund, reservation.OrganizationQuota, reservation.SelfQuota)
		if err != nil {
			return OrganizationWalletReservationResult{}, 0, err
		}
	}
	previousStatus := reservation.Status
	reservationUpdate := tx.Model(&OrganizationWalletReservation{}).
		Where("id = ? AND status = ? AND reserved_quota = ? AND organization_quota = ? AND self_quota = ?", reservation.Id, previousStatus, reservation.ReservedQuota, reservation.OrganizationQuota, reservation.SelfQuota).
		Update("status", OrganizationWalletReservationRefunded)
	if reservationUpdate.Error != nil {
		return OrganizationWalletReservationResult{}, 0, reservationUpdate.Error
	}
	if reservationUpdate.RowsAffected != 1 {
		return OrganizationWalletReservationResult{}, 0, ErrOrganizationReservationState
	}
	reservation.Status = OrganizationWalletReservationRefunded
	relatedLedgerId := reservation.ReserveLedgerId
	ledger := organizationQuotaLedger(operation, reservation.ReservedQuota, 0, reservation.OrganizationQuota, refund.UserQuotaAfter, account.Quota, refund.RecoverableQuotaAfter, &relatedLedgerId)
	accounting, err := commitOrganizationAccounting(tx, operation, ledger, "organization.wallet.refund", "wallet_reservation", strconv.FormatInt(reservation.Id, 10))
	if err != nil {
		return OrganizationWalletReservationResult{}, 0, err
	}
	return OrganizationWalletReservationResult{Reservation: *reservation, Accounting: accounting}, reservation.ReservedQuota, nil
}
