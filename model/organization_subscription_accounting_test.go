package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type organizationSubscriptionAccountingFixture struct {
	organization Organization
	user         User
	subscription UserSubscription
}

func setupOrganizationSubscriptionAccountingTest(t *testing.T, totalAmount int64, consumptionLimit int64) organizationSubscriptionAccountingFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "organization-subscription.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Organization{},
		&OrganizationMemberFund{},
		&User{},
		&SubscriptionPlan{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
	))

	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	organization := Organization{
		Name:          "Organization Subscription Accounting Test",
		Status:        OrganizationStatusActive,
		OwnerUserId:   8801,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	user := User{
		Id:                 8802,
		Username:           "organization-subscription-member",
		Password:           "unused-password",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organization.Id,
		OrganizationRole:   OrganizationRoleMember,
		OrganizationStatus: OrganizationMemberStatusActive,
		AffCode:            "organization-subscription-member-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	limit := consumptionLimit
	require.NoError(t, db.Create(&OrganizationMemberFund{
		OrganizationId:   organization.Id,
		UserId:           user.Id,
		ConsumptionLimit: &limit,
	}).Error)
	plan := SubscriptionPlan{
		Title:            "Organization Subscription Plan",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      totalAmount,
		QuotaResetPeriod: SubscriptionResetNever,
		Enabled:          true,
	}
	require.NoError(t, db.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() {
		InvalidateSubscriptionPlanCache(plan.Id)
	})
	now := time.Now().Unix()
	subscription := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: totalAmount,
		StartTime: now - 60, EndTime: now + 3600, Status: "active", Source: "test",
	}
	require.NoError(t, db.Create(&subscription).Error)

	return organizationSubscriptionAccountingFixture{
		organization: organization,
		user:         user,
		subscription: subscription,
	}
}

func TestOrganizationSubscriptionPreConsumeMetersLimitAndReplays(t *testing.T) {
	fixture := setupOrganizationSubscriptionAccountingTest(t, 100, 50)

	first, err := PreConsumeOrganizationUserSubscription(
		fixture.organization.Id,
		"organization-subscription-preconsume",
		fixture.user.Id,
		"test-model",
		0,
		40,
	)
	require.NoError(t, err)
	assert.EqualValues(t, 40, first.PreConsumed)
	assert.EqualValues(t, 40, first.AmountUsedAfter)

	replayed, err := PreConsumeOrganizationUserSubscription(
		fixture.organization.Id,
		"organization-subscription-preconsume",
		fixture.user.Id,
		"test-model",
		0,
		45,
	)
	require.NoError(t, err)
	assert.EqualValues(t, 40, replayed.PreConsumed)
	assert.EqualValues(t, 40, replayed.AmountUsedAfter)

	_, err = PreConsumeOrganizationUserSubscription(
		fixture.organization.Id,
		"organization-subscription-over-limit",
		fixture.user.Id,
		"test-model",
		0,
		20,
	)
	require.ErrorIs(t, err, ErrOrganizationConsumptionLimit)

	var subscription UserSubscription
	require.NoError(t, DB.First(&subscription, fixture.subscription.Id).Error)
	assert.EqualValues(t, 40, subscription.AmountUsed)
	var fund OrganizationMemberFund
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	assert.EqualValues(t, 40, fund.ConsumedQuota)
	var records int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Count(&records).Error)
	assert.EqualValues(t, 1, records)
}

func TestOrganizationSubscriptionSettlementAndRefundAreAtomic(t *testing.T) {
	fixture := setupOrganizationSubscriptionAccountingTest(t, 100, 200)
	requestID := "organization-subscription-lifecycle"
	_, err := PreConsumeOrganizationUserSubscription(
		fixture.organization.Id,
		requestID,
		fixture.user.Id,
		"test-model",
		0,
		40,
	)
	require.NoError(t, err)
	require.NoError(t, PostConsumeOrganizationUserSubscriptionDelta(
		fixture.organization.Id,
		fixture.user.Id,
		fixture.subscription.Id,
		30,
	))

	err = PostConsumeOrganizationUserSubscriptionDelta(
		fixture.organization.Id,
		fixture.user.Id,
		fixture.subscription.Id,
		40,
	)
	require.Error(t, err)

	var subscription UserSubscription
	require.NoError(t, DB.First(&subscription, fixture.subscription.Id).Error)
	assert.EqualValues(t, 70, subscription.AmountUsed)
	var fund OrganizationMemberFund
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	assert.EqualValues(t, 70, fund.ConsumedQuota)

	limit := int64(75)
	require.NoError(t, DB.Model(&OrganizationMemberFund{}).
		Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).
		Update("consumption_limit", limit).Error)
	err = PostConsumeOrganizationUserSubscriptionDelta(
		fixture.organization.Id,
		fixture.user.Id,
		fixture.subscription.Id,
		10,
	)
	require.ErrorIs(t, err, ErrOrganizationConsumptionLimit)
	require.NoError(t, DB.First(&subscription, fixture.subscription.Id).Error)
	assert.EqualValues(t, 70, subscription.AmountUsed)
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	assert.EqualValues(t, 70, fund.ConsumedQuota)

	require.NoError(t, DB.Model(&Organization{}).Where("id = ?", fixture.organization.Id).
		Update("status", OrganizationStatusDisabled).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", fixture.user.Id).
		Update("organization_status", OrganizationMemberStatusDisabled).Error)
	require.NoError(t, PostConsumeOrganizationUserSubscriptionDelta(
		fixture.organization.Id,
		fixture.user.Id,
		fixture.subscription.Id,
		-30,
	))
	require.NoError(t, RefundSubscriptionPreConsume(requestID))
	require.NoError(t, RefundSubscriptionPreConsume(requestID))

	require.NoError(t, DB.First(&subscription, fixture.subscription.Id).Error)
	assert.Zero(t, subscription.AmountUsed)
	require.NoError(t, DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	assert.Zero(t, fund.ConsumedQuota)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", requestID).First(&record).Error)
	assert.Equal(t, "refunded", record.Status)
}

type legacySubscriptionPreConsumeRecord struct {
	Id                 int    `gorm:"primaryKey"`
	RequestId          string `gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `gorm:"index"`
	UserSubscriptionId int    `gorm:"index"`
	PreConsumed        int64  `gorm:"type:bigint;not null;default:0"`
	Status             string `gorm:"type:varchar(32);index"`
	CreatedAt          int64  `gorm:"bigint"`
	UpdatedAt          int64  `gorm:"bigint;index"`
}

func (legacySubscriptionPreConsumeRecord) TableName() string {
	return "subscription_pre_consume_records"
}

func TestSubscriptionPreConsumeMigrationPreservesLegacyReplayAndRefund(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy-subscription.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &legacySubscriptionPreConsumeRecord{}))

	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	now := time.Now().Unix()
	subscription := UserSubscription{
		UserId: 9901, AmountTotal: 100, AmountUsed: 40,
		StartTime: now - 60, EndTime: now + 3600, Status: "active", Source: "legacy",
	}
	require.NoError(t, db.Create(&subscription).Error)
	require.NoError(t, db.Create(&legacySubscriptionPreConsumeRecord{
		RequestId:          "legacy-subscription-preconsume",
		UserId:             subscription.UserId,
		UserSubscriptionId: subscription.Id,
		PreConsumed:        40,
		Status:             "consumed",
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error)

	require.NoError(t, db.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	assert.True(t, db.Migrator().HasColumn(&SubscriptionPreConsumeRecord{}, "OrganizationId"))
	var migrated SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", "legacy-subscription-preconsume").First(&migrated).Error)
	assert.Nil(t, migrated.OrganizationId)

	replayed, err := PreConsumeUserSubscription(
		"legacy-subscription-preconsume",
		subscription.UserId,
		"test-model",
		0,
		50,
	)
	require.NoError(t, err)
	assert.EqualValues(t, 40, replayed.PreConsumed)
	assert.EqualValues(t, 40, replayed.AmountUsedAfter)
	require.NoError(t, RefundSubscriptionPreConsume("legacy-subscription-preconsume"))
	require.NoError(t, RefundSubscriptionPreConsume("legacy-subscription-preconsume"))

	require.NoError(t, db.First(&subscription, subscription.Id).Error)
	assert.Zero(t, subscription.AmountUsed)
	require.NoError(t, db.Where("request_id = ?", "legacy-subscription-preconsume").First(&migrated).Error)
	assert.Equal(t, "refunded", migrated.Status)
}
