package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOrganizationFundingTest(t *testing.T) (model.Organization, model.User) {
	t.Helper()
	db := setupRegistrationTestDB(t)
	organization := model.Organization{
		Name:          "Organization Funding Test",
		Status:        model.OrganizationStatusActive,
		OwnerUserId:   1,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id, Quota: 0}).Error)
	user := model.User{
		Username:           "organization-funding-member",
		Password:           "password123",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organization.Id,
		OrganizationRole:   model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
		Quota:              100,
		AffCode:            "organization-funding-member-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{OrganizationId: organization.Id, UserId: user.Id, RecoverableQuota: 40}).Error)
	return organization, user
}

func TestOrganizationWalletFundingUsesDurableReservationLifecycle(t *testing.T) {
	organization, user := setupOrganizationFundingTest(t)
	funding, err := NewOrganizationWalletFunding(organization.Id, user.Id, "funding-request-1", model.OrganizationAccountingActor{
		Kind:   model.OrganizationAccountingActorSystem,
		Policy: "relay_billing",
	})
	require.NoError(t, err)
	require.NoError(t, funding.PreConsume(60))

	reservation, ok := funding.Reservation()
	require.True(t, ok)
	assert.EqualValues(t, 40, reservation.OrganizationQuota)
	assert.EqualValues(t, 20, reservation.SelfQuota)
	require.NoError(t, funding.Settle(-10))
	require.NoError(t, funding.Settle(0))
	require.NoError(t, funding.Refund())
	require.NoError(t, funding.Refund())

	var refreshed model.User
	require.NoError(t, model.DB.First(&refreshed, user.Id).Error)
	assert.Equal(t, 100, refreshed.Quota)
}

func TestOrganizationWalletFundingMapsInsufficientQuota(t *testing.T) {
	organization, user := setupOrganizationFundingTest(t)
	funding, err := NewOrganizationWalletFunding(organization.Id, user.Id, "funding-request-2", model.OrganizationAccountingActor{
		Kind:   model.OrganizationAccountingActorSystem,
		Policy: "relay_billing",
	})
	require.NoError(t, err)

	err = funding.PreConsume(101)
	require.ErrorIs(t, err, ErrInsufficientWalletQuota)
	require.NoError(t, funding.PreConsume(100))
	reservation, ok := funding.Reservation()
	require.True(t, ok)
	assert.EqualValues(t, 100, reservation.ReservedQuota)
}

func TestOrganizationWalletFundingExtendsOpenReservation(t *testing.T) {
	organization, user := setupOrganizationFundingTest(t)
	funding, err := NewOrganizationWalletFunding(organization.Id, user.Id, "funding-request-extend", model.OrganizationAccountingActor{
		Kind:   model.OrganizationAccountingActorSystem,
		Policy: "relay_billing",
	})
	require.NoError(t, err)
	require.NoError(t, funding.PreConsume(40))
	require.NoError(t, funding.ReserveTo(70))

	reservation, ok := funding.Reservation()
	require.True(t, ok)
	assert.EqualValues(t, 70, reservation.ReservedQuota)
	assert.EqualValues(t, 40, reservation.OrganizationQuota)
	assert.EqualValues(t, 30, reservation.SelfQuota)

	var refreshed model.User
	require.NoError(t, model.DB.First(&refreshed, user.Id).Error)
	assert.Equal(t, 30, refreshed.Quota)
	require.NoError(t, funding.Settle(0))
	assert.Equal(t, model.OrganizationWalletReservationSettled, mustOrganizationFundingReservation(t, funding).Status)
}

func TestOrganizationWalletFundingLazilyReservesPositiveZeroEstimate(t *testing.T) {
	organization, user := setupOrganizationFundingTest(t)
	funding, err := NewOrganizationWalletFunding(organization.Id, user.Id, "funding-request-zero", model.OrganizationAccountingActor{
		Kind:   model.OrganizationAccountingActorSystem,
		Policy: "relay_billing",
	})
	require.NoError(t, err)
	require.NoError(t, funding.PreConsume(0))
	_, reserved := funding.Reservation()
	assert.False(t, reserved)
	require.NoError(t, funding.Settle(25))

	reservation := mustOrganizationFundingReservation(t, funding)
	assert.EqualValues(t, 25, reservation.ReservedQuota)
	assert.Equal(t, model.OrganizationWalletReservationSettled, reservation.Status)
	var refreshed model.User
	require.NoError(t, model.DB.First(&refreshed, user.Id).Error)
	assert.Equal(t, 75, refreshed.Quota)
}

func mustOrganizationFundingReservation(t *testing.T, funding *OrganizationWalletFunding) model.OrganizationWalletReservation {
	t.Helper()
	reservation, ok := funding.Reservation()
	require.True(t, ok)
	return reservation
}

func TestNewBillingSessionUsesOrganizationWalletForWalletPreference(t *testing.T) {
	organization, user := setupOrganizationFundingTest(t)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:         user.Id,
		OrganizationId: organization.Id,
		RequestId:      "organization-billing-session",
		IsPlayground:   true,
		TokenUnlimited: true,
		UserSetting:    dto.UserSetting{BillingPreference: "wallet_only"},
	}
	session, apiErr := NewBillingSession(ctx, info, 25)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	_, isOrganizationWallet := session.funding.(*OrganizationWalletFunding)
	assert.True(t, isOrganizationWallet)
	assert.Equal(t, 25, session.GetPreConsumedQuota())
	_, reserved := session.funding.(*OrganizationWalletFunding).Reservation()
	assert.True(t, reserved, "organization funding must not use the zero-cost trust bypass")
}

func TestNewBillingSessionHonorsOrganizationSubscriptionPreferences(t *testing.T) {
	for _, preference := range []string{"subscription_only", "subscription_first"} {
		t.Run(preference, func(t *testing.T) {
			organization, user := setupOrganizationFundingTest(t)
			require.NoError(t, model.DB.AutoMigrate(
				&model.SubscriptionPlan{},
				&model.UserSubscription{},
				&model.SubscriptionPreConsumeRecord{},
			))
			plan := model.SubscriptionPlan{
				Title:         "Organization subscription entitlement",
				TotalAmount:   100,
				Enabled:       true,
				DurationUnit:  model.SubscriptionDurationMonth,
				DurationValue: 1,
			}
			require.NoError(t, model.DB.Create(&plan).Error)
			now := model.GetDBTimestamp()
			subscription := model.UserSubscription{
				UserId: user.Id, PlanId: plan.Id, AmountTotal: plan.TotalAmount,
				StartTime: now - 60, EndTime: now + 3600, Status: "active", Source: "test",
			}
			require.NoError(t, model.DB.Create(&subscription).Error)

			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(nil)
			info := &relaycommon.RelayInfo{
				UserId: user.Id, OrganizationId: organization.Id,
				RequestId:       "organization-subscription-" + preference,
				OriginModelName: "test-model", IsPlayground: true, TokenUnlimited: true,
				UserSetting: dto.UserSetting{BillingPreference: preference},
			}
			session, apiErr := NewBillingSession(ctx, info, 25)
			require.Nil(t, apiErr)
			require.NotNil(t, session)
			_, isSubscription := session.funding.(*SubscriptionFunding)
			assert.True(t, isSubscription)
			assert.Equal(t, BillingSourceSubscription, info.BillingSource)
			assert.Equal(t, 25, session.GetPreConsumedQuota())

			var refreshedSubscription model.UserSubscription
			require.NoError(t, model.DB.First(&refreshedSubscription, subscription.Id).Error)
			assert.EqualValues(t, 25, refreshedSubscription.AmountUsed)
			var refreshedUser model.User
			require.NoError(t, model.DB.First(&refreshedUser, user.Id).Error)
			assert.Equal(t, 100, refreshedUser.Quota)
			var walletReservations int64
			require.NoError(t, model.DB.Model(&model.OrganizationWalletReservation{}).Count(&walletReservations).Error)
			assert.Zero(t, walletReservations)
		})
	}
}
