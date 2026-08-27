package model

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOrganizationFundTopUpTest(t *testing.T, poolQuota int64) {
	t.Helper()
	setupOrganizationAccountingTest(t, poolQuota, 0, 0)
	require.NoError(t, DB.AutoMigrate(&TopUp{}))
	prefix := "org-fund-topup-" + strings.ReplaceAll(t.Name(), "/", "-")
	require.NoError(t, DB.Where("trade_no LIKE ?", prefix+"%").Delete(&TopUp{}).Error)
	t.Cleanup(func() {
		_ = DB.Where("trade_no LIKE ?", prefix+"%").Delete(&TopUp{}).Error
	})
}

func organizationFundTopUpOrder(t *testing.T, suffix, provider string, amount int64, money float64) *TopUp {
	t.Helper()
	order := &TopUp{
		UserId:          organizationAccountingTestOwnerId,
		OrganizationId:  organizationAccountingTestOrganizationId + 999,
		Amount:          amount,
		Money:           money,
		TradeNo:         "org-fund-topup-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + suffix,
		PaymentMethod:   provider,
		PaymentProvider: provider,
		TopUpTarget:     TopUpTargetOrganization,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())
	assert.Equal(t, organizationAccountingTestOrganizationId, order.OrganizationId)
	return order
}

func TestOrganizationFundTopUpSettlementAcrossPaymentProviders(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	testCases := []struct {
		name       string
		provider   string
		amount     int64
		money      float64
		wantCredit int64
		settle     func(*TopUp) error
	}{
		{
			name: "epay", provider: PaymentProviderEpay, amount: 2, money: 2, wantCredit: 20,
			settle: func(order *TopUp) error {
				_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
				return err
			},
		},
		{
			name: "stripe", provider: PaymentProviderStripe, amount: 3, money: 3, wantCredit: 30,
			settle: func(order *TopUp) error {
				return Recharge(order.TradeNo, "cus_org_fund", "127.0.0.1")
			},
		},
		{
			name: "creem", provider: PaymentProviderCreem, amount: 40, money: 4, wantCredit: 40,
			settle: func(order *TopUp) error {
				return RechargeCreem(order.TradeNo, "owner@example.com", "Owner", "127.0.0.1")
			},
		},
		{
			name: "waffo", provider: PaymentProviderWaffo, amount: 5, money: 5, wantCredit: 50,
			settle: func(order *TopUp) error {
				return RechargeWaffo(order.TradeNo, "127.0.0.1")
			},
		},
		{
			name: "waffo pancake", provider: PaymentProviderWaffoPancake, amount: 6, money: 6, wantCredit: 60,
			settle: func(order *TopUp) error {
				return RechargeWaffoPancake(order.TradeNo)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setupOrganizationFundTopUpTest(t, 7)
			order := organizationFundTopUpOrder(t, testCase.name, testCase.provider, testCase.amount, testCase.money)

			require.NoError(t, testCase.settle(order))
			var account OrganizationFundAccount
			require.NoError(t, DB.Where("organization_id = ?", organizationAccountingTestOrganizationId).First(&account).Error)
			assert.Equal(t, int64(7)+testCase.wantCredit, account.Quota)
			var owner User
			require.NoError(t, DB.First(&owner, organizationAccountingTestOwnerId).Error)
			assert.Zero(t, owner.Quota)

			var ledger OrganizationQuotaLedger
			require.NoError(t, DB.Where("source_id = ?", order.TradeNo).First(&ledger).Error)
			assert.Equal(t, OrganizationLedgerFundCredit, ledger.Operation)
			assert.Equal(t, "payment_topup", ledger.SourceType)
			assert.Zero(t, ledger.UserId)
			assert.Zero(t, ledger.ActorUserId)
			require.NotNil(t, ledger.InitiatorUserId)
			assert.Equal(t, order.UserId, *ledger.InitiatorUserId)
			assert.Equal(t, testCase.wantCredit, ledger.PoolQuotaDelta)
			assert.Equal(t, int64(7)+testCase.wantCredit, ledger.PoolQuotaAfter)

			var operation OrganizationQuotaOperation
			require.NoError(t, DB.Where("ledger_id = ?", ledger.Id).First(&operation).Error)
			assert.Equal(t, OrganizationAccountingActorSystem, operation.ActorKind)
			assert.Zero(t, operation.ActorUserId)
			require.NotNil(t, operation.InitiatorUserId)
			assert.Equal(t, order.UserId, *operation.InitiatorUserId)

			var audit OrganizationAuditEvent
			auditRequestID := "topup:" + order.TradeNo
			if len(auditRequestID) > 64 {
				auditRequestID = organizationAccountingFingerprint("topup-request", testCase.provider, order.TradeNo)
			}
			require.NoError(t, DB.Where("request_id = ?", auditRequestID).First(&audit).Error)
			assert.Equal(t, "organization.fund.credit", audit.Action)
			assert.Equal(t, fmt.Sprint(organizationAccountingTestOrganizationId), audit.TargetId)
			assert.Zero(t, audit.ActorUserId)
			require.NotNil(t, audit.InitiatorUserId)
			assert.Equal(t, order.UserId, *audit.InitiatorUserId)
			var metadata map[string]interface{}
			require.NoError(t, common.UnmarshalJsonStr(audit.Metadata, &metadata))
			assert.EqualValues(t, order.UserId, metadata["initiator_user_id"])

			require.NoError(t, testCase.settle(order))
			require.NoError(t, DB.Where("organization_id = ?", organizationAccountingTestOrganizationId).First(&account).Error)
			assert.Equal(t, int64(7)+testCase.wantCredit, account.Quota)
			assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
			assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationAuditEvent{}))
			assert.EqualValues(t, 1, countOrganizationAccountingRows(t, &OrganizationQuotaOperation{}))
			require.NoError(t, DB.Where("source_id = ?", order.TradeNo).First(&ledger).Error)
			require.NotNil(t, ledger.InitiatorUserId)
			assert.Equal(t, order.UserId, *ledger.InitiatorUserId)
		})
	}
}

func TestOrganizationFundTopUpAuthorizationAndPaidOrderSnapshot(t *testing.T) {
	setupOrganizationFundTopUpTest(t, 0)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	require.NoError(t, ValidateOrganizationFundTopUpCapacity(organizationAccountingTestOwnerId, 10))
	require.NoError(t, ValidateOrganizationFundTopUpCapacity(organizationAccountingTestAdminId, 10))
	require.ErrorIs(t, ValidateOrganizationFundTopUpCapacity(organizationAccountingTestMemberId, 10), ErrOrganizationAccountingForbidden)

	memberOrder := TopUp{
		UserId: organizationAccountingTestMemberId, Amount: 1, Money: 1,
		TradeNo: "org-fund-topup-member-forbidden", PaymentProvider: PaymentProviderEpay,
		TopUpTarget: TopUpTargetOrganization, Status: common.TopUpStatusPending,
	}
	require.ErrorIs(t, memberOrder.Insert(), ErrOrganizationAccountingForbidden)

	order := organizationFundTopUpOrder(t, "paid-snapshot", PaymentProviderEpay, 2, 2)
	require.NoError(t, DB.Model(&Organization{}).Where("id = ?", organizationAccountingTestOrganizationId).
		Update("status", OrganizationStatusDisabled).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", organizationAccountingTestOwnerId).
		Update("organization_status", OrganizationMemberStatusDisabled).Error)

	alreadyDone, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	var account OrganizationFundAccount
	require.NoError(t, DB.Where("organization_id = ?", organizationAccountingTestOrganizationId).First(&account).Error)
	assert.EqualValues(t, 20, account.Quota)
}

func TestOrganizationFundTopUpOverflowRollsBackOrderAndAccounting(t *testing.T) {
	setupOrganizationFundTopUpTest(t, math.MaxInt64-5)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	order := organizationFundTopUpOrder(t, "overflow", PaymentProviderEpay, 1, 1)
	_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.ErrorIs(t, err, ErrOrganizationFundOverflow)

	persisted := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, persisted)
	assert.Equal(t, common.TopUpStatusPending, persisted.Status)
	var account OrganizationFundAccount
	require.NoError(t, DB.Where("organization_id = ?", organizationAccountingTestOrganizationId).First(&account).Error)
	assert.Equal(t, int64(math.MaxInt64-5), account.Quota)
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaLedger{}))
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationAuditEvent{}))
	assert.Zero(t, countOrganizationAccountingRows(t, &OrganizationQuotaOperation{}))
}

func TestGetOrganizationTopUpsIncludesOnlyOrganizationFundOrders(t *testing.T) {
	setupOrganizationFundTopUpTest(t, 0)
	organizationFundTopUpOrder(t, "owner-history", PaymentProviderEpay, 1, 1)
	adminOrder := TopUp{
		UserId: organizationAccountingTestAdminId, Amount: 2, Money: 2,
		TradeNo:       "org-fund-topup-" + strings.ReplaceAll(t.Name(), "/", "-") + "-admin-history",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		TopUpTarget: TopUpTargetOrganization, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, adminOrder.Insert())
	personalOrder := TopUp{
		UserId: organizationAccountingTestOwnerId, Amount: 3, Money: 3,
		TradeNo:       "org-fund-topup-" + strings.ReplaceAll(t.Name(), "/", "-") + "-personal-history",
		PaymentMethod: PaymentProviderEpay, PaymentProvider: PaymentProviderEpay,
		TopUpTarget: TopUpTargetPersonal, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, personalOrder.Insert())
	secondOwnerId := organizationAccountingTestOwnerId + 9
	secondOrganizationId := organizationAccountingTestOrganizationId + 1
	secondOwner := User{
		Id: secondOwnerId, Username: "organization-history-second-owner", Password: "unused-password",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		OrganizationId: secondOrganizationId, OrganizationRole: OrganizationRoleOwner,
		OrganizationStatus: OrganizationMemberStatusActive, AffCode: "org-history-second-owner-aff",
	}
	require.NoError(t, DB.Create(&secondOwner).Error)
	secondOrganization := Organization{
		Id: secondOrganizationId, Name: "Second History Organization", Status: OrganizationStatusActive,
		OwnerUserId: secondOwnerId, PolicyVersion: 1,
	}
	require.NoError(t, DB.Create(&secondOrganization).Error)
	require.NoError(t, DB.Create(&OrganizationFundAccount{OrganizationId: secondOrganizationId}).Error)
	secondOrganizationOrder := TopUp{
		UserId: secondOwnerId, Amount: 4, Money: 4,
		TradeNo:       "org-fund-topup-" + strings.ReplaceAll(t.Name(), "/", "-") + "-second-organization-history",
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		TopUpTarget: TopUpTargetOrganization, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, secondOrganizationOrder.Insert())

	orders, total, err := GetOrganizationTopUps(
		organizationAccountingTestOrganizationId,
		"%history%",
		&common.PageInfo{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, orders, 2)
	for _, order := range orders {
		assert.Equal(t, TopUpTargetOrganization, order.TopUpTarget)
		assert.Equal(t, organizationAccountingTestOrganizationId, order.OrganizationId)
		assert.Equal(t, "Accounting Test Organization", order.OrganizationName)
	}

	personalOrders, personalTotal, err := GetUserTopUps(
		organizationAccountingTestOwnerId,
		&common.PageInfo{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 1, personalTotal)
	require.Len(t, personalOrders, 1)
	assert.Equal(t, personalOrder.TradeNo, personalOrders[0].TradeNo)
	assert.Empty(t, personalOrders[0].OrganizationName)
	encodedPersonalOrders, err := common.Marshal(personalOrders)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedPersonalOrders), "organization_name")

	searchedPersonalOrders, searchedPersonalTotal, err := SearchUserTopUps(
		organizationAccountingTestOwnerId,
		"%personal-history%",
		&common.PageInfo{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 1, searchedPersonalTotal)
	require.Len(t, searchedPersonalOrders, 1)
	assert.Empty(t, searchedPersonalOrders[0].OrganizationName)

	globalOrders, globalTotal, err := SearchAllTopUps(
		"%history%",
		&common.PageInfo{Page: 1, PageSize: 20},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 4, globalTotal)
	globalNames := make(map[string]string, len(globalOrders))
	for _, order := range globalOrders {
		globalNames[order.TradeNo] = order.OrganizationName
	}
	assert.Equal(t, "Accounting Test Organization", globalNames[adminOrder.TradeNo])
	assert.Equal(t, "Second History Organization", globalNames[secondOrganizationOrder.TradeNo])
	assert.Empty(t, globalNames[personalOrder.TradeNo])

	allOrders, _, err := GetAllTopUps(&common.PageInfo{Page: 1, PageSize: 100})
	require.NoError(t, err)
	allNames := make(map[string]string, len(allOrders))
	for _, order := range allOrders {
		allNames[order.TradeNo] = order.OrganizationName
	}
	assert.Equal(t, "Accounting Test Organization", allNames[adminOrder.TradeNo])
	assert.Equal(t, "Second History Organization", allNames[secondOrganizationOrder.TradeNo])
}
