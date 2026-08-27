package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUserTopUpPermissionOnlyRestrictsMembers(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 0, 0)
	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", false).Error)

	require.ErrorIs(t, ValidateUserTopUpPermission(organizationAccountingTestMemberId), ErrOrganizationMemberTopupForbidden)
	require.NoError(t, ValidateUserTopUpPermission(organizationAccountingTestOwnerId))
	require.NoError(t, ValidateUserTopUpPermission(organizationAccountingTestAdminId))

	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", true).Error)
	require.NoError(t, ValidateUserTopUpPermission(organizationAccountingTestMemberId))
}

func TestLegacyQuotaMutatorsFailClosedForOrganizationUsers(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 100, 40)
	require.ErrorIs(t, IncreaseUserQuota(organizationAccountingTestMemberId, 10, true), ErrOrganizationLedgerRequired)
	require.ErrorIs(t, DecreaseUserQuota(organizationAccountingTestMemberId, 10, true), ErrOrganizationLedgerRequired)
	reserved, err := TryReserveUserQuota(organizationAccountingTestMemberId, 10)
	require.ErrorIs(t, err, ErrOrganizationLedgerRequired)
	assert.False(t, reserved)

	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 100, userQuota)
	assert.EqualValues(t, 40, recoverableQuota)
	assert.Zero(t, consumedQuota)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func TestRedeemOrganizationMemberPolicyAndLedger(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 0, 0)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	t.Cleanup(func() {
		_ = DB.Where("name = ?", "organization-redemption").Unscoped().Delete(&Redemption{}).Error
	})

	redemption := Redemption{
		Name:        "organization-redemption",
		Key:         "organization-redemption-code-01",
		Status:      common.RedemptionCodeStatusEnabled,
		Quota:       500,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&redemption).Error)
	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", false).Error)

	_, err := Redeem(redemption.Key, organizationAccountingTestMemberId)
	require.ErrorIs(t, err, ErrRedeemFailed)
	require.NoError(t, DB.First(&redemption, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, redemption.Status)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))

	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", true).Error)
	credited, err := Redeem(redemption.Key, organizationAccountingTestMemberId)
	require.NoError(t, err)
	assert.Equal(t, 500, credited)

	var user User
	require.NoError(t, DB.First(&user, organizationAccountingTestMemberId).Error)
	assert.Equal(t, 500, user.Quota)
	var memberFund OrganizationMemberFund
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", organizationAccountingTestOrganizationId, user.Id).First(&memberFund).Error)
	assert.Zero(t, memberFund.RecoverableQuota)

	var ledger OrganizationQuotaLedger
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", organizationAccountingTestOrganizationId, user.Id).First(&ledger).Error)
	assert.Equal(t, OrganizationLedgerWalletCredit, ledger.Operation)
	assert.Equal(t, "redemption", ledger.SourceType)
	assert.Equal(t, fmt.Sprintf("%d", redemption.Id), ledger.SourceId)
	assert.EqualValues(t, 500, ledger.UserQuotaDelta)
}

func TestTransferAffQuotaToQuotaUsesOrganizationLedger(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 100, 0)
	transferQuota := common.QuotaFromFloat(common.QuotaPerUnit)
	require.Positive(t, transferQuota)
	require.NoError(t, DB.Model(&User{}).
		Where("id = ?", organizationAccountingTestMemberId).
		Updates(map[string]interface{}{
			"aff_quota":   transferQuota,
			"aff_history": transferQuota,
		}).Error)

	var user User
	require.NoError(t, DB.First(&user, organizationAccountingTestMemberId).Error)
	require.NoError(t, user.TransferAffQuotaToQuota(transferQuota))
	assert.Zero(t, user.AffQuota)
	assert.Equal(t, 100+transferQuota, user.Quota)

	require.NoError(t, DB.First(&user, organizationAccountingTestMemberId).Error)
	assert.Zero(t, user.AffQuota)
	assert.Equal(t, 100+transferQuota, user.Quota)
	var memberFund OrganizationMemberFund
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", organizationAccountingTestOrganizationId, user.Id).First(&memberFund).Error)
	assert.Zero(t, memberFund.RecoverableQuota)

	var ledgers []OrganizationQuotaLedger
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", organizationAccountingTestOrganizationId, user.Id).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	assert.Equal(t, OrganizationLedgerWalletCredit, ledgers[0].Operation)
	assert.Equal(t, "referral_transfer", ledgers[0].SourceType)
	assert.EqualValues(t, transferQuota, ledgers[0].UserQuotaDelta)

	require.Error(t, user.TransferAffQuotaToQuota(transferQuota))
	require.NoError(t, DB.First(&user, organizationAccountingTestMemberId).Error)
	assert.Zero(t, user.AffQuota)
	assert.Equal(t, 100+transferQuota, user.Quota)
	assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func TestTransferAffQuotaToQuotaRejectsStaleReferralBalanceWithoutCreditingWallet(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 100, 0)
	transferQuota := common.QuotaFromFloat(common.QuotaPerUnit)
	require.Positive(t, transferQuota)

	staleUser := User{
		Id:              organizationAccountingTestMemberId,
		AffQuota:        transferQuota,
		AffHistoryQuota: transferQuota,
	}
	require.Error(t, staleUser.TransferAffQuotaToQuota(transferQuota))

	var persisted User
	require.NoError(t, DB.First(&persisted, organizationAccountingTestMemberId).Error)
	assert.Zero(t, persisted.AffQuota)
	assert.Equal(t, 100, persisted.Quota)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaOperation{}))
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
}

func TestOrganizationCheckinCreditsLedgerExactlyOnce(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 0, 0)
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	require.NoError(t, DB.Where("user_id = ?", organizationAccountingTestMemberId).Delete(&Checkin{}).Error)
	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", false).Error)

	setting := operation_setting.GetCheckinSetting()
	previous := *setting
	setting.Enabled = true
	setting.MinQuota = 50
	setting.MaxQuota = 50
	t.Cleanup(func() { *setting = previous })

	checkin, err := UserCheckin(organizationAccountingTestMemberId)
	require.NoError(t, err)
	assert.Equal(t, 50, checkin.QuotaAwarded)
	_, err = UserCheckin(organizationAccountingTestMemberId)
	require.Error(t, err)

	var user User
	require.NoError(t, DB.First(&user, organizationAccountingTestMemberId).Error)
	assert.Equal(t, 50, user.Quota)
	var ledgers []OrganizationQuotaLedger
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	assert.Equal(t, OrganizationLedgerWalletCredit, ledgers[0].Operation)
	assert.Equal(t, "checkin", ledgers[0].SourceType)
	assert.EqualValues(t, 50, ledgers[0].UserQuotaDelta)
}

func TestPaidOrganizationTopUpSettlesAfterMemberPolicyChanges(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 0, 0)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", false).Error)

	order := TopUp{
		UserId:          organizationAccountingTestMemberId,
		Amount:          2,
		Money:           2,
		TradeNo:         "organization-paid-topup-1",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(&order).Error)

	alreadyDone, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	alreadyDone, err = RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)

	var user User
	require.NoError(t, DB.First(&user, organizationAccountingTestMemberId).Error)
	assert.Equal(t, 200, user.Quota)
	var ledgers []OrganizationQuotaLedger
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	assert.Equal(t, OrganizationLedgerWalletCredit, ledgers[0].Operation)
	assert.Equal(t, "payment_topup", ledgers[0].SourceType)
	assert.Equal(t, order.TradeNo, ledgers[0].SourceId)
}

func TestOrganizationBalanceSubscriptionDebitIgnoresTopUpPolicy(t *testing.T) {
	setupOrganizationAccountingTest(t, 0, 100, 60)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	require.NoError(t, DB.Model(&Organization{}).
		Where("id = ?", organizationAccountingTestOrganizationId).
		Update("allow_member_topup", false).Error)

	allowBalancePay := true
	plan := SubscriptionPlan{
		Title:           "Organization Balance Plan",
		PriceAmount:     5,
		Currency:        "USD",
		DurationUnit:    SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		AllowBalancePay: &allowBalancePay,
		TotalAmount:     1000,
	}
	require.NoError(t, DB.Create(&plan).Error)
	require.NoError(t, PurchaseSubscriptionWithBalance(organizationAccountingTestMemberId, plan.Id))

	_, userQuota, recoverableQuota, consumedQuota := getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 50, userQuota)
	assert.EqualValues(t, 10, recoverableQuota)
	assert.EqualValues(t, 50, consumedQuota)
	var ledger OrganizationQuotaLedger
	require.NoError(t, DB.Where("user_id = ? AND operation = ?", organizationAccountingTestMemberId, OrganizationLedgerWalletDebit).First(&ledger).Error)
	assert.Equal(t, "subscription_balance", ledger.SourceType)
	assert.EqualValues(t, -50, ledger.UserQuotaDelta)
	assert.EqualValues(t, -50, ledger.RecoverableQuotaDelta)

	insufficientPlan := SubscriptionPlan{
		Title:           "Organization Insufficient Balance Plan",
		PriceAmount:     6,
		Currency:        "USD",
		DurationUnit:    SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		AllowBalancePay: &allowBalancePay,
		TotalAmount:     1000,
	}
	require.NoError(t, DB.Create(&insufficientPlan).Error)
	require.Error(t, PurchaseSubscriptionWithBalance(organizationAccountingTestMemberId, insufficientPlan.Id))
	_, userQuota, recoverableQuota, consumedQuota = getOrganizationAccountingBalances(t, organizationAccountingTestMemberId)
	assert.EqualValues(t, 50, userQuota)
	assert.EqualValues(t, 10, recoverableQuota)
	assert.EqualValues(t, 50, consumedQuota)
}
