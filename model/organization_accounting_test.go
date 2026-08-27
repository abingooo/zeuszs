package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	organizationAccountingTestOrganizationId = 9100
	organizationAccountingTestOwnerId        = 9101
	organizationAccountingTestAdminId        = 9102
	organizationAccountingTestMemberId       = 9103
)

func setupOrganizationAccountingTest(t *testing.T, poolQuota int64, memberQuota int, recoverableQuota int64) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&Organization{},
		&OrganizationFundAccount{},
		&OrganizationMemberFund{},
		&OrganizationQuotaOperation{},
		&OrganizationWalletReservation{},
		&OrganizationQuotaLedger{},
		&OrganizationAuditEvent{},
	))

	tables := []string{
		"organization_audit_events",
		"organization_quota_ledgers",
		"organization_wallet_reservations",
		"organization_quota_operations",
		"organization_member_funds",
		"organization_fund_accounts",
		"organizations",
	}
	for _, table := range tables {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	require.NoError(t, DB.Where("id >= ? AND id <= ?", organizationAccountingTestOwnerId, organizationAccountingTestMemberId+10).Unscoped().Delete(&User{}).Error)

	users := []User{
		{
			Id:                 organizationAccountingTestOwnerId,
			Username:           "organization-accounting-owner",
			Password:           "unused-password",
			Role:               common.RoleCommonUser,
			Status:             common.UserStatusEnabled,
			OrganizationId:     organizationAccountingTestOrganizationId,
			OrganizationRole:   OrganizationRoleOwner,
			OrganizationStatus: OrganizationMemberStatusActive,
			AffCode:            "org-accounting-owner-aff",
		},
		{
			Id:                 organizationAccountingTestAdminId,
			Username:           "organization-accounting-admin",
			Password:           "unused-password",
			Role:               common.RoleCommonUser,
			Status:             common.UserStatusEnabled,
			OrganizationId:     organizationAccountingTestOrganizationId,
			OrganizationRole:   OrganizationRoleAdmin,
			OrganizationStatus: OrganizationMemberStatusActive,
			AffCode:            "org-accounting-admin-aff",
		},
		{
			Id:                 organizationAccountingTestMemberId,
			Username:           "organization-accounting-member",
			Password:           "unused-password",
			Role:               common.RoleCommonUser,
			Status:             common.UserStatusEnabled,
			OrganizationId:     organizationAccountingTestOrganizationId,
			OrganizationRole:   OrganizationRoleMember,
			OrganizationStatus: OrganizationMemberStatusActive,
			Quota:              memberQuota,
			AffCode:            "org-accounting-member-aff",
		},
	}
	for index := range users {
		require.NoError(t, DB.Create(&users[index]).Error)
	}
	organization := Organization{
		Id:               organizationAccountingTestOrganizationId,
		Name:             "Accounting Test Organization",
		Status:           OrganizationStatusActive,
		OwnerUserId:      organizationAccountingTestOwnerId,
		AllowMemberTopup: true,
		PolicyVersion:    1,
	}
	require.NoError(t, DB.Create(&organization).Error)
	require.NoError(t, DB.Create(&OrganizationFundAccount{
		OrganizationId: organization.Id,
		Quota:          poolQuota,
	}).Error)
	require.NoError(t, DB.Create(&OrganizationMemberFund{
		OrganizationId:   organization.Id,
		UserId:           organizationAccountingTestMemberId,
		RecoverableQuota: recoverableQuota,
	}).Error)

	t.Cleanup(func() {
		for _, table := range tables {
			_ = DB.Exec("DELETE FROM " + table).Error
		}
		_ = DB.Where("id >= ? AND id <= ?", organizationAccountingTestOwnerId, organizationAccountingTestMemberId+10).Unscoped().Delete(&User{}).Error
	})
}

func organizationAccountingTestActor(userId int) OrganizationAccountingActor {
	return OrganizationAccountingActor{
		Kind:   OrganizationAccountingActorUser,
		UserId: userId,
		Policy: "organization_quota_management",
	}
}

func organizationAccountingSystemActor(policy string) OrganizationAccountingActor {
	return OrganizationAccountingActor{Kind: OrganizationAccountingActorSystem, Policy: policy}
}

func TestOrganizationAccountingRejectsForgedOwnerRole(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, 0, 0)
	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("owner_user_id", organizationAccountingTestAdminId).Error)

	_, err := AllocateOrganizationQuota(OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         10,
		SourceType:     "allocation",
		SourceId:       "forged-owner",
		IdempotencyKey: "forged-owner-allocation",
		RequestId:      "forged-owner-request",
		Actor:          organizationAccountingTestActor(organizationAccountingTestOwnerId),
	})
	assert.ErrorIs(t, err, ErrOrganizationAccountingForbidden)

	pool, wallet, recoverable, _ := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 100, pool)
	assert.Zero(t, wallet)
	assert.Zero(t, recoverable)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func getOrganizationAccountingBalances(t *testing.T, userId int) (int64, int64, int64, int64) {
	t.Helper()
	var account OrganizationFundAccount
	require.NoError(t, DB.Where("organization_id = ?", organizationAccountingTestOrganizationId).First(&account).Error)
	var user User
	require.NoError(t, DB.Where("id = ?", userId).First(&user).Error)
	var memberFund OrganizationMemberFund
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", organizationAccountingTestOrganizationId, userId).First(&memberFund).Error)
	return account.Quota, int64(user.Quota), memberFund.RecoverableQuota, memberFund.ConsumedQuota
}

func countOrganizationAccountingRows(t *testing.T, value interface{}) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(value).Count(&count).Error)
	return count
}

func TestLockOrganizationAccountingScopesTxValidatesAndDeduplicatesScopes(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, 0, 0)
	secondOrganizationId := organizationAccountingTestOrganizationId + 1
	require.NoError(t, DB.Create(&Organization{
		Id:            secondOrganizationId,
		Name:          "Second Accounting Scope",
		Status:        OrganizationStatusActive,
		OwnerUserId:   organizationAccountingTestOwnerId + 20,
		PolicyVersion: 1,
	}).Error)
	require.NoError(t, DB.Create(&OrganizationFundAccount{OrganizationId: secondOrganizationId, Quota: 25}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return LockOrganizationAccountingScopesTx(tx, []int{secondOrganizationId, organizationAccountingTestOrganizationId, secondOrganizationId})
	}))
	require.ErrorIs(t, DB.Transaction(func(tx *gorm.DB) error {
		return LockOrganizationAccountingScopesTx(tx, []int{organizationAccountingTestOrganizationId, 0})
	}), ErrOrganizationAccountingInvalid)

	require.NoError(t, DB.Where("organization_id = ?", secondOrganizationId).Delete(&OrganizationFundAccount{}).Error)
	require.ErrorIs(t, DB.Transaction(func(tx *gorm.DB) error {
		return LockOrganizationAccountingScopesTx(tx, []int{organizationAccountingTestOrganizationId, secondOrganizationId})
	}), gorm.ErrRecordNotFound)
}

func TestCreditOrganizationFundIsIdempotentAndAudited(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, 0, 0)
	params := OrganizationFundCreditParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		Amount:         50,
		SourceType:     "platform_adjustment",
		SourceId:       "adjustment-1",
		IdempotencyKey: "fund-credit-1",
		RequestId:      "request-fund-credit-1",
		Actor:          organizationAccountingSystemActor("platform_adjustment"),
	}

	first, err := CreditOrganizationFund(params)
	require.NoError(t, err)
	assert.False(t, first.AlreadyApplied)
	assert.EqualValues(t, 150, first.PoolQuotaAfter)

	params.RequestId = "request-fund-credit-retry"
	second, err := CreditOrganizationFund(params)
	require.NoError(t, err)
	assert.True(t, second.AlreadyApplied)
	assert.Equal(t, first.LedgerId, second.LedgerId)
	assert.EqualValues(t, 150, second.PoolQuotaAfter)

	params.Amount = 51
	_, err = CreditOrganizationFund(params)
	require.ErrorIs(t, err, ErrOrganizationAccountingIdempotency)
	poolQuota, _, _, _ := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 150, poolQuota)
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationAuditEvent{}))
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaOperation{}))
}

func TestDebitOrganizationWalletIsNonnegativeAndIdempotent(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 150, 100)
	params := OrganizationWalletDebitParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         120,
		SourceType:     "subscription_balance",
		SourceId:       "subscription-order-1",
		IdempotencyKey: "subscription-order-1",
		RequestId:      "subscription-request-1",
		Actor:          organizationAccountingSystemActor("subscription_balance_purchase"),
	}

	first, err := DebitOrganizationUserWalletTx(DB, params)
	require.NoError(t, err)
	assert.False(t, first.AlreadyApplied)
	assert.EqualValues(t, 30, first.UserQuotaAfter)
	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 30, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 120, consumedQuota)

	params.RequestId = "subscription-request-retry"
	second, err := DebitOrganizationUserWalletTx(DB, params)
	require.NoError(t, err)
	assert.True(t, second.AlreadyApplied)
	assert.Equal(t, first.LedgerId, second.LedgerId)
	_, userQuota, recoverableQuota, consumedQuota = getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 30, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 120, consumedQuota)

	params.Amount = 121
	_, err = DebitOrganizationUserWalletTx(DB, params)
	require.ErrorIs(t, err, ErrOrganizationAccountingIdempotency)
	_, err = DebitOrganizationUserWalletTx(DB, OrganizationWalletDebitParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         31,
		SourceType:     "subscription_balance",
		SourceId:       "subscription-order-2",
		IdempotencyKey: "subscription-order-2",
		RequestId:      "subscription-request-2",
		Actor:          organizationAccountingSystemActor("subscription_balance_purchase"),
	})
	require.ErrorIs(t, err, ErrOrganizationUserQuotaInsufficient)
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func TestAllocationAndRecoveryPreserveSelfFundedQuota(t *testing.T) {
	setupOrganizationAccountingTest(t, 500, 0, 0)
	actor := organizationAccountingTestActor(organizationAccountingTestAdminId)
	selfCredit, err := CreditOrganizationUserWallet(OrganizationWalletCreditParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         20,
		SourceType:     "member_topup",
		SourceId:       "topup-1",
		IdempotencyKey: "topup-1",
		RequestId:      "request-topup-1",
		Actor:          organizationAccountingSystemActor("payment_settlement"),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 20, selfCredit.UserQuotaAfter)
	assert.Zero(t, selfCredit.RecoverableQuotaAfter)

	allocation, err := AllocateOrganizationQuota(OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         100,
		SourceType:     "organization_pool",
		SourceId:       "allocation-1",
		IdempotencyKey: "allocation-1",
		RequestId:      "request-allocation-1",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 400, allocation.PoolQuotaAfter)
	assert.EqualValues(t, 120, allocation.UserQuotaAfter)
	assert.EqualValues(t, 100, allocation.RecoverableQuotaAfter)

	_, err = RecoverOrganizationQuota(OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         110,
		SourceType:     "organization_pool",
		SourceId:       "recovery-too-large",
		IdempotencyKey: "recovery-too-large",
		RequestId:      "request-recovery-too-large",
		Actor:          actor,
	})
	require.ErrorIs(t, err, ErrOrganizationRecoverableInsufficient)
	poolQuota, userQuota, recoverableQuota, _ := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 400, poolQuota)
	assert.EqualValues(t, 120, userQuota)
	assert.EqualValues(t, 100, recoverableQuota)

	recovery, err := RecoverOrganizationQuota(OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         100,
		SourceType:     "organization_pool",
		SourceId:       "recovery-1",
		IdempotencyKey: "recovery-1",
		RequestId:      "request-recovery-1",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 500, recovery.PoolQuotaAfter)
	assert.EqualValues(t, 20, recovery.UserQuotaAfter)
	assert.Zero(t, recovery.RecoverableQuotaAfter)
}

func TestWalletReservationRestoresExactSourceSplit(t *testing.T) {
	setupOrganizationAccountingTest(t, 500, 150, 100)
	actor := organizationAccountingSystemActor("relay_billing")

	reserved, err := ReserveOrganizationWalletQuota(OrganizationWalletReserveParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         120,
		RequestId:      "relay-request-1",
		IdempotencyKey: "relay-request-1:reserve",
		SourceType:     "relay",
		SourceId:       "relay-request-1",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 100, reserved.Reservation.OrganizationQuota)
	assert.EqualValues(t, 20, reserved.Reservation.SelfQuota)
	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 30, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 120, consumedQuota)

	settled, err := SettleOrganizationWalletQuota(OrganizationWalletSettleParams{
		ReservationId:  reserved.Reservation.Id,
		ActualQuota:    80,
		IdempotencyKey: "relay-request-1:settle",
		RequestId:      "relay-request-1-settle",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.Equal(t, OrganizationWalletReservationSettled, settled.Reservation.Status)
	assert.EqualValues(t, 80, settled.Reservation.OrganizationQuota)
	assert.Zero(t, settled.Reservation.SelfQuota)
	_, userQuota, recoverableQuota, consumedQuota = getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 70, userQuota)
	assert.EqualValues(t, 20, recoverableQuota)
	assert.EqualValues(t, 80, consumedQuota)

	refunded, err := RefundOrganizationWalletQuota(OrganizationWalletRefundParams{
		ReservationId:  reserved.Reservation.Id,
		IdempotencyKey: "relay-request-1:refund",
		RequestId:      "relay-request-1-refund",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.Equal(t, OrganizationWalletReservationRefunded, refunded.Reservation.Status)
	_, userQuota, recoverableQuota, consumedQuota = getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 150, userQuota)
	assert.EqualValues(t, 100, recoverableQuota)
	assert.Zero(t, consumedQuota)

	var ledgers []OrganizationQuotaLedger
	require.NoError(t, DB.Order("id asc").Find(&ledgers).Error)
	require.Len(t, ledgers, 3)
	assert.EqualValues(t, -100, ledgers[0].RecoverableQuotaDelta)
	assert.EqualValues(t, 20, ledgers[1].RecoverableQuotaDelta)
	assert.EqualValues(t, 80, ledgers[2].RecoverableQuotaDelta)
}

func TestAdditionalWalletReservationReplayUsesStableFingerprint(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 300, 100)
	actor := organizationAccountingSystemActor("relay_billing")
	reserved, err := ReserveOrganizationWalletQuota(OrganizationWalletReserveParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         100,
		RequestId:      "additional-reserve-base",
		IdempotencyKey: "additional-reserve-base:reserve",
		SourceType:     "relay",
		SourceId:       "additional-reserve-base",
		Actor:          actor,
	})
	require.NoError(t, err)

	firstParams := OrganizationWalletReservationIncreaseParams{
		ReservationId:  reserved.Reservation.Id,
		Amount:         20,
		IdempotencyKey: "additional-reserve:first",
		RequestId:      "additional-reserve-first",
		Actor:          actor,
	}
	first, err := ReserveAdditionalOrganizationWalletQuota(firstParams)
	require.NoError(t, err)
	assert.False(t, first.Accounting.AlreadyApplied)
	assert.EqualValues(t, 120, first.Reservation.ReservedQuota)

	second, err := ReserveAdditionalOrganizationWalletQuota(OrganizationWalletReservationIncreaseParams{
		ReservationId:  reserved.Reservation.Id,
		Amount:         10,
		IdempotencyKey: "additional-reserve:second",
		RequestId:      "additional-reserve-second",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 130, second.Reservation.ReservedQuota)

	replayed, err := ReserveAdditionalOrganizationWalletQuota(firstParams)
	require.NoError(t, err)
	assert.True(t, replayed.Accounting.AlreadyApplied)
	assert.Equal(t, first.Accounting.LedgerId, replayed.Accounting.LedgerId)
	assert.EqualValues(t, 130, replayed.Reservation.ReservedQuota)

	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 170, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 130, consumedQuota)
	assert.EqualValues(t, 3, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func TestSettledWalletAdjustmentRequiresExpectedQuotaAndSupportsCycles(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 200, 100)
	actor := organizationAccountingSystemActor("async_task_billing")
	reserved, err := ReserveOrganizationWalletQuota(OrganizationWalletReserveParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         120,
		RequestId:      "async-task-1",
		IdempotencyKey: "async-task-1:reserve",
		SourceType:     "async_task",
		SourceId:       "task-1",
		Actor:          actor,
	})
	require.NoError(t, err)
	_, err = SettleOrganizationWalletQuota(OrganizationWalletSettleParams{
		ReservationId:  reserved.Reservation.Id,
		ActualQuota:    120,
		IdempotencyKey: "async-task-1:settle",
		RequestId:      "async-task-1-settle",
		Actor:          actor,
	})
	require.NoError(t, err)

	up, err := AdjustSettledOrganizationWalletQuota(OrganizationWalletAdjustmentParams{
		ReservationId:  reserved.Reservation.Id,
		ExpectedQuota:  120,
		ActualQuota:    150,
		IdempotencyKey: "async-task-1:adjust:1",
		RequestId:      "async-task-1-adjust-1",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 150, up.Reservation.SettledQuota)
	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 50, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 150, consumedQuota)

	_, err = AdjustSettledOrganizationWalletQuota(OrganizationWalletAdjustmentParams{
		ReservationId:  reserved.Reservation.Id,
		ExpectedQuota:  120,
		ActualQuota:    140,
		IdempotencyKey: "async-task-1:stale",
		RequestId:      "async-task-1-stale",
		Actor:          actor,
	})
	require.ErrorIs(t, err, ErrOrganizationReservationState)

	down, err := AdjustSettledOrganizationWalletQuota(OrganizationWalletAdjustmentParams{
		ReservationId:  reserved.Reservation.Id,
		ExpectedQuota:  150,
		ActualQuota:    120,
		IdempotencyKey: "async-task-1:adjust:2",
		RequestId:      "async-task-1-adjust-2",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 120, down.Reservation.SettledQuota)

	_, err = AdjustSettledOrganizationWalletQuota(OrganizationWalletAdjustmentParams{
		ReservationId:  reserved.Reservation.Id,
		ExpectedQuota:  120,
		ActualQuota:    150,
		IdempotencyKey: "async-task-1:adjust:1",
		RequestId:      "async-task-1-old-replay",
		Actor:          actor,
	})
	require.ErrorIs(t, err, ErrOrganizationReservationState)

	cycled, err := AdjustSettledOrganizationWalletQuota(OrganizationWalletAdjustmentParams{
		ReservationId:  reserved.Reservation.Id,
		ExpectedQuota:  120,
		ActualQuota:    150,
		IdempotencyKey: "async-task-1:adjust:3",
		RequestId:      "async-task-1-adjust-3",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 150, cycled.Reservation.SettledQuota)
	_, userQuota, recoverableQuota, consumedQuota = getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 50, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 150, consumedQuota)
}

func TestSettledWalletAdjustmentWithUnchangedQuotaCommitsIdempotently(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 200, 100)
	actor := organizationAccountingSystemActor("async_task_billing")
	reserved, err := ReserveOrganizationWalletQuota(OrganizationWalletReserveParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         120,
		RequestId:      "unchanged-adjustment-reserve",
		IdempotencyKey: "unchanged-adjustment:reserve",
		SourceType:     "async_task",
		SourceId:       "unchanged-adjustment",
		Actor:          actor,
	})
	require.NoError(t, err)
	_, err = SettleOrganizationWalletQuota(OrganizationWalletSettleParams{
		ReservationId:  reserved.Reservation.Id,
		ActualQuota:    120,
		IdempotencyKey: "unchanged-adjustment:settle",
		RequestId:      "unchanged-adjustment-settle",
		Actor:          actor,
	})
	require.NoError(t, err)

	params := OrganizationWalletAdjustmentParams{
		ReservationId:  reserved.Reservation.Id,
		ExpectedQuota:  120,
		ActualQuota:    120,
		IdempotencyKey: "unchanged-adjustment:adjust",
		RequestId:      "unchanged-adjustment-adjust",
		Actor:          actor,
	}
	adjusted, err := AdjustSettledOrganizationWalletQuota(params)
	require.NoError(t, err)
	assert.False(t, adjusted.Accounting.AlreadyApplied)
	assert.EqualValues(t, 120, adjusted.Reservation.SettledQuota)
	assert.Equal(t, OrganizationWalletReservationSettled, adjusted.Reservation.Status)

	replayed, err := AdjustSettledOrganizationWalletQuota(params)
	require.NoError(t, err)
	assert.True(t, replayed.Accounting.AlreadyApplied)

	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 80, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.EqualValues(t, 120, consumedQuota)
	var ledger OrganizationQuotaLedger
	require.NoError(t, DB.Where("idempotency_key = ?", params.IdempotencyKey).First(&ledger).Error)
	assert.Equal(t, OrganizationLedgerAdjust, ledger.Operation)
	assert.Zero(t, ledger.UserQuotaDelta)
	assert.Zero(t, ledger.RecoverableQuotaDelta)
}

func TestAllocationFailureRollsBackEveryAccountingRecord(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, common.MaxQuota-10, 0)

	_, err := AllocateOrganizationQuota(OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         20,
		SourceType:     "organization_pool",
		SourceId:       "allocation-overflow",
		IdempotencyKey: "allocation-overflow",
		RequestId:      "request-allocation-overflow",
		Actor:          organizationAccountingTestActor(organizationAccountingTestOwnerId),
	})
	require.ErrorIs(t, err, ErrOrganizationUserQuotaLimit)
	poolQuota, userQuota, recoverableQuota, _ := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 100, poolQuota)
	assert.EqualValues(t, common.MaxQuota-10, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationAuditEvent{}))
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaOperation{}))
}

func TestConcurrentAllocationsCannotOverdrawSQLitePool(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, 0, 0)
	secondMemberId := organizationAccountingTestMemberId + 1
	secondMember := User{
		Id:                 secondMemberId,
		Username:           "organization-accounting-member-2",
		Password:           "unused-password",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organizationAccountingTestOrganizationId,
		OrganizationRole:   OrganizationRoleMember,
		OrganizationStatus: OrganizationMemberStatusActive,
		AffCode:            "org-accounting-member-2-aff",
	}
	require.NoError(t, DB.Create(&secondMember).Error)
	require.NoError(t, DB.Create(&OrganizationMemberFund{OrganizationId: organizationAccountingTestOrganizationId, UserId: secondMemberId}).Error)

	start := make(chan struct{})
	errorsByMember := make(map[int]error)
	var mu sync.Mutex
	var wait sync.WaitGroup
	for _, memberId := range []int{organizationAccountingTestMemberId, secondMemberId} {
		memberId := memberId
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := AllocateOrganizationQuota(OrganizationQuotaTransferParams{
				OrganizationId: organizationAccountingTestOrganizationId,
				UserId:         memberId,
				Amount:         80,
				SourceType:     "organization_pool",
				SourceId:       fmt.Sprintf("concurrent-allocation-%d", memberId),
				IdempotencyKey: fmt.Sprintf("concurrent-allocation-%d", memberId),
				RequestId:      fmt.Sprintf("concurrent-request-%d", memberId),
				Actor:          organizationAccountingTestActor(organizationAccountingTestAdminId),
			})
			mu.Lock()
			errorsByMember[memberId] = err
			mu.Unlock()
		}()
	}
	close(start)
	wait.Wait()

	successCount := 0
	insufficientCount := 0
	for _, err := range errorsByMember {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrOrganizationFundInsufficient):
			insufficientCount++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, insufficientCount)
	var account OrganizationFundAccount
	require.NoError(t, DB.Where("organization_id = ?", organizationAccountingTestOrganizationId).First(&account).Error)
	assert.EqualValues(t, 20, account.Quota)
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func TestConcurrentDuplicateAllocationAppliesExactlyOnce(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, 0, 0)
	params := OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         60,
		SourceType:     "organization_pool",
		SourceId:       "duplicate-allocation",
		IdempotencyKey: "duplicate-allocation",
		RequestId:      "duplicate-request",
		Actor:          organizationAccountingTestActor(organizationAccountingTestAdminId),
	}

	start := make(chan struct{})
	results := make(chan OrganizationAccountingResult, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := AllocateOrganizationQuota(params)
			results <- result
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	alreadyApplied := 0
	for result := range results {
		if result.AlreadyApplied {
			alreadyApplied++
		}
	}
	assert.Equal(t, 1, alreadyApplied)
	poolQuota, userQuota, recoverableQuota, _ := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 40, poolQuota)
	assert.EqualValues(t, 60, userQuota)
	assert.EqualValues(t, 60, recoverableQuota)
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationAuditEvent{}))
}

func TestZeroSettlementCanBeRefundedIdempotently(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 100, 0)
	actor := organizationAccountingSystemActor("relay_billing")
	reserved, err := ReserveOrganizationWalletQuota(OrganizationWalletReserveParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         50,
		RequestId:      "zero-settlement-reserve",
		IdempotencyKey: "zero-settlement:reserve",
		SourceType:     "relay",
		SourceId:       "zero-settlement",
		Actor:          actor,
	})
	require.NoError(t, err)
	settled, err := SettleOrganizationWalletQuota(OrganizationWalletSettleParams{
		ReservationId:  reserved.Reservation.Id,
		ActualQuota:    0,
		IdempotencyKey: "zero-settlement:settle",
		RequestId:      "zero-settlement-settle",
		Actor:          actor,
	})
	require.NoError(t, err)
	assert.Zero(t, settled.Reservation.ReservedQuota)
	assert.Equal(t, OrganizationWalletReservationSettled, settled.Reservation.Status)

	refundParams := OrganizationWalletRefundParams{
		ReservationId:  reserved.Reservation.Id,
		IdempotencyKey: "zero-settlement:refund",
		RequestId:      "zero-settlement-refund",
		Actor:          actor,
	}
	refunded, err := RefundOrganizationWalletQuota(refundParams)
	require.NoError(t, err)
	assert.Equal(t, OrganizationWalletReservationRefunded, refunded.Reservation.Status)
	assert.Zero(t, refunded.Accounting.UserQuotaAfter-100)

	duplicate, err := RefundOrganizationWalletQuota(refundParams)
	require.NoError(t, err)
	assert.True(t, duplicate.Accounting.AlreadyApplied)
	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 100, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.Zero(t, consumedQuota)
}

func TestWalletReservationEnforcesMemberConsumptionLimit(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 100, 0)
	limit := int64(50)
	require.NoError(t, DB.Model(&OrganizationMemberFund{}).
		Where("organization_id = ? AND user_id = ?", organizationAccountingTestOrganizationId, organizationAccountingTestMemberId).
		Update("consumption_limit", limit).Error)

	_, err := ReserveOrganizationWalletQuota(OrganizationWalletReserveParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestMemberId,
		Amount:         60,
		RequestId:      "limit-request",
		IdempotencyKey: "limit-request:reserve",
		SourceType:     "relay",
		SourceId:       "limit-request",
		Actor:          organizationAccountingSystemActor("relay_billing"),
	})
	require.ErrorIs(t, err, ErrOrganizationConsumptionLimit)
	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 100, userQuota)
	assert.Zero(t, recoverableQuota)
	assert.Zero(t, consumedQuota)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaOperation{}))
}

func TestOrganizationAdminCannotAllocateToAnotherAdmin(t *testing.T) {
	setupOrganizationAccountingTest(t, 100, 0, 0)

	_, err := AllocateOrganizationQuota(OrganizationQuotaTransferParams{
		OrganizationId: organizationAccountingTestOrganizationId,
		UserId:         organizationAccountingTestOwnerId,
		Amount:         10,
		SourceType:     "organization_pool",
		SourceId:       "admin-target",
		IdempotencyKey: "admin-target",
		RequestId:      "request-admin-target",
		Actor:          organizationAccountingTestActor(organizationAccountingTestAdminId),
	})
	require.ErrorIs(t, err, ErrOrganizationTargetNotMember)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}
