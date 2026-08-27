package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type organizationTaskBillingFixture struct {
	organization model.Organization
	user         model.User
	token        model.Token
	channel      model.Channel
}

func setupOrganizationTaskBillingTest(t *testing.T, quota int, recoverableQuota int64) organizationTaskBillingFixture {
	t.Helper()
	db := setupRegistrationTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Channel{}, &model.Log{}, &model.BillingLogOutbox{}, &model.UserSubscription{}))
	organization := model.Organization{
		Name:          "Organization Task Billing Test",
		Status:        model.OrganizationStatusActive,
		OwnerUserId:   7001,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	user := model.User{
		Id:                 7002,
		Username:           "organization-task-member",
		Password:           "password",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organization.Id,
		OrganizationRole:   model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
		Quota:              quota,
		AffCode:            "organization-task-member-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{
		OrganizationId: organization.Id, UserId: user.Id, RecoverableQuota: recoverableQuota,
	}).Error)
	token := model.Token{
		Id: 7003, UserId: user.Id, Key: "organization-task-token", Name: "organization-task-token",
		Status: common.TokenStatusEnabled, RemainQuota: 1000,
	}
	require.NoError(t, db.Create(&token).Error)
	channel := model.Channel{Id: 7004, Name: "organization-task-channel", Key: "test", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	return organizationTaskBillingFixture{organization: organization, user: user, token: token, channel: channel}
}

func reserveSettledOrganizationTaskWallet(t *testing.T, fixture organizationTaskBillingFixture, quota int, sourceID string) model.OrganizationWalletReservation {
	t.Helper()
	actor := model.OrganizationAccountingActor{Kind: model.OrganizationAccountingActorSystem, Policy: "async_task_billing"}
	reserved, err := model.ReserveOrganizationWalletQuota(model.OrganizationWalletReserveParams{
		OrganizationId: fixture.organization.Id,
		UserId:         fixture.user.Id,
		Amount:         int64(quota),
		RequestId:      sourceID,
		IdempotencyKey: sourceID + ":reserve",
		SourceType:     "async_task",
		SourceId:       sourceID,
		Actor:          actor,
	})
	require.NoError(t, err)
	settled, err := model.SettleOrganizationWalletQuota(model.OrganizationWalletSettleParams{
		ReservationId:  reserved.Reservation.Id,
		ActualQuota:    int64(quota),
		IdempotencyKey: sourceID + ":settle",
		RequestId:      sourceID + "-settle",
		Actor:          actor,
	})
	require.NoError(t, err)
	return settled.Reservation
}

func seedOrganizationTaskUsage(t *testing.T, fixture organizationTaskBillingFixture, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fixture.user.Id).Updates(map[string]interface{}{
		"used_quota": quota, "request_count": 1,
	}).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.channel.Id).Update("used_quota", quota).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).Updates(map[string]interface{}{
		"remain_quota": 1000 - quota, "used_quota": quota,
	}).Error)
}

func seedOrganizationSubscriptionTaskUsage(t *testing.T, fixture organizationTaskBillingFixture, quota int) {
	t.Helper()
	seedOrganizationTaskUsage(t, fixture, quota)
	require.NoError(t, model.DB.Model(&model.OrganizationMemberFund{}).
		Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).
		Update("consumed_quota", quota).Error)
}

func loadOrganizationTaskBillingBalances(t *testing.T, fixture organizationTaskBillingFixture) (model.User, model.OrganizationMemberFund, model.Token, model.Channel) {
	t.Helper()
	var user model.User
	var fund model.OrganizationMemberFund
	var token model.Token
	var channel model.Channel
	require.NoError(t, model.DB.First(&user, fixture.user.Id).Error)
	require.NoError(t, model.DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	require.NoError(t, model.DB.First(&token, fixture.token.Id).Error)
	require.NoError(t, model.DB.First(&channel, fixture.channel.Id).Error)
	return user, fund, token, channel
}

func TestOrganizationTaskAdjustmentRejectsStaleDuplicatesAndSupportsCycles(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	reservation := reserveSettledOrganizationTaskWallet(t, fixture, 120, "organization-task-adjust")
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "organization-task-adjust", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{OriginModelName: "test-model"},
		PrivateData: model.TaskPrivateData{
			OrganizationReservationId: reservation.Id,
			TokenId:                   fixture.token.Id,
			BillingSource:             BillingSourceWallet,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Task
	var stalePoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.ID).Error)

	RecalculateTaskQuota(context.Background(), &firstPoll, 150, "first adjustment")
	RecalculateTaskQuota(context.Background(), &stalePoll, 150, "stale duplicate")

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 150, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 50, user.Quota)
	assert.Equal(t, 150, user.UsedQuota)
	assert.EqualValues(t, 150, fund.ConsumedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 850, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	assert.EqualValues(t, 150, channel.UsedQuota)

	RecalculateTaskQuota(context.Background(), &persisted, 120, "cycle down")
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	RecalculateTaskQuota(context.Background(), &persisted, 150, "cycle up")
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 150, persisted.Quota)
	assert.EqualValues(t, 3, persisted.BillingRevision)
	user, fund, token, channel = loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 50, user.Quota)
	assert.EqualValues(t, 150, fund.ConsumedQuota)
	assert.Equal(t, 850, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	assert.EqualValues(t, 150, channel.UsedQuota)
}

func TestOrganizationTaskRefundAppliesSideEffectsOnce(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	reservation := reserveSettledOrganizationTaskWallet(t, fixture, 120, "organization-task-refund")
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "organization-task-refund", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusFailure, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			OrganizationReservationId: reservation.Id,
			TokenId:                   fixture.token.Id,
			BillingSource:             BillingSourceWallet,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Task
	var stalePoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.ID).Error)

	assert.True(t, RefundTaskQuota(context.Background(), &firstPoll, "failed"))
	assert.True(t, RefundTaskQuota(context.Background(), &stalePoll, "stale duplicate"))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Zero(t, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	var logCount int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", fixture.user.Id, model.LogTypeRefund).Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)
}

func TestOrganizationTaskBillingMutationRejectsMismatchedTaskIdentity(t *testing.T) {
	tests := []struct {
		name       string
		mutateTask func(*model.Task)
		mutate     func(*model.OrganizationTaskBillingMutationParams)
	}{
		{
			name: "token",
			mutate: func(params *model.OrganizationTaskBillingMutationParams) {
				params.TokenId = 0
			},
		},
		{
			name: "channel",
			mutate: func(params *model.OrganizationTaskBillingMutationParams) {
				params.ChannelId = 0
			},
		},
		{
			name: "legacy wallet marker",
			mutateTask: func(task *model.Task) {
				task.LegacyOrganizationWallet = true
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setupOrganizationTaskBillingTest(t, 200, 100)
			reservation := reserveSettledOrganizationTaskWallet(t, fixture, 120, "organization-task-identity-"+test.name)
			seedOrganizationTaskUsage(t, fixture, 120)
			task := model.Task{
				TaskID: "organization-task-identity-" + test.name, UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
				Quota: 120, Status: model.TaskStatusFailure, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
				PrivateData: model.TaskPrivateData{
					OrganizationReservationId: reservation.Id,
					TokenId:                   fixture.token.Id,
					BillingSource:             BillingSourceWallet,
				},
			}
			if test.mutateTask != nil {
				test.mutateTask(&task)
			}
			require.NoError(t, model.DB.Create(&task).Error)
			operationID := "task-identity-" + test.name
			params := model.OrganizationTaskBillingMutationParams{
				TaskId: task.ID, UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
				OrganizationReservationId: reservation.Id, TokenId: fixture.token.Id, ChannelId: fixture.channel.Id,
				ExpectedQuota: 120, ActualQuota: 0, OperationId: operationID,
			}
			if test.mutate != nil {
				test.mutate(&params)
			}

			_, err := model.ApplyOrganizationTaskBillingMutation(params)
			assert.ErrorIs(t, err, model.ErrOrganizationIdentityInvalid)

			var persisted model.Task
			require.NoError(t, model.DB.First(&persisted, task.ID).Error)
			assert.Equal(t, 120, persisted.Quota)
			assert.Zero(t, persisted.BillingRevision)
			user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
			assert.Equal(t, 80, user.Quota)
			assert.Equal(t, 120, user.UsedQuota)
			assert.Zero(t, fund.RecoverableQuota)
			assert.EqualValues(t, 120, fund.ConsumedQuota)
			assert.Equal(t, 880, token.RemainQuota)
			assert.Equal(t, 120, token.UsedQuota)
			assert.EqualValues(t, 120, channel.UsedQuota)
			var ledgerCount int64
			require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
				Where("idempotency_key = ?", operationID).
				Count(&ledgerCount).Error)
			assert.Zero(t, ledgerCount)
		})
	}
}

func TestOrganizationTaskAdjustmentRollsBackOnTokenFailureAndRetriesAtomically(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	reservation := reserveSettledOrganizationTaskWallet(t, fixture, 120, "organization-task-atomic-adjust")
	seedOrganizationTaskUsage(t, fixture, 120)
	useTaskBillingMiniRedis(t)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
	})
	tokenCacheKey := seedTaskBillingTokenCache(t, fixture.token.Id, fixture.token.Key, 880)
	require.NoError(t, common.RDB.HSet(t.Context(), tokenCacheKey, "UsedQuota", 120).Err())

	task := model.Task{
		TaskID: "organization-task-atomic-adjust", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{OriginModelName: "test-model"},
		PrivateData: model.TaskPrivateData{
			OrganizationReservationId: reservation.Id,
			TokenId:                   fixture.token.Id,
			BillingSource:             BillingSourceWallet,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_organization_task_token_update
		BEFORE UPDATE OF remain_quota ON tokens
		WHEN OLD.id = 7003
		BEGIN
			SELECT RAISE(ABORT, 'forced organization task token failure');
		END;
	`).Error)

	RecalculateTaskQuota(context.Background(), &task, 150, "forced token failure")

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 120, persisted.Quota)
	assert.Zero(t, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
	cachedRemainQuota, cachedUsedQuota := getTaskBillingTokenCacheQuota(t, tokenCacheKey)
	assert.Equal(t, 880, cachedRemainQuota)
	assert.Equal(t, 120, cachedUsedQuota)
	var adjustmentLedgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("idempotency_key = ?", taskOrganizationAccountingID(&task, "adjust:0:120:150")).
		Count(&adjustmentLedgers).Error)
	assert.Zero(t, adjustmentLedgers)
	var billingLogs int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ?", fixture.user.Id).Count(&billingLogs).Error)
	assert.Zero(t, billingLogs)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_organization_task_token_update").Error)
	RecalculateTaskQuota(context.Background(), &task, 150, "retry after token failure")
	var stale model.Task
	require.NoError(t, model.DB.First(&stale, task.ID).Error)
	RecalculateTaskQuota(context.Background(), &stale, 150, "idempotent retry")

	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 150, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel = loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 50, user.Quota)
	assert.Equal(t, 150, user.UsedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 150, fund.ConsumedQuota)
	assert.Equal(t, 850, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	assert.EqualValues(t, 150, channel.UsedQuota)
	cachedRemainQuota, cachedUsedQuota = getTaskBillingTokenCacheQuota(t, tokenCacheKey)
	assert.Equal(t, 850, cachedRemainQuota)
	assert.Equal(t, 150, cachedUsedQuota)
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("idempotency_key = ?", taskOrganizationAccountingID(&task, "adjust:0:120:150")).
		Count(&adjustmentLedgers).Error)
	assert.EqualValues(t, 1, adjustmentLedgers)
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ?", fixture.user.Id).Count(&billingLogs).Error)
	assert.EqualValues(t, 1, billingLogs)
}

func TestOrganizationSubscriptionTaskRefundMetersOrganizationAndAppliesSideEffectsOnce(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 0)
	subscription := model.UserSubscription{
		UserId: fixture.user.Id, AmountTotal: 1000, AmountUsed: 120,
		Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(&subscription).Error)
	seedOrganizationSubscriptionTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "organization-subscription-refund", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusFailure, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			BillingSource:                   BillingSourceSubscription,
			SubscriptionId:                  subscription.Id,
			OrganizationSubscriptionMetered: true,
			TokenId:                         fixture.token.Id,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Task
	var stalePoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.ID).Error)

	assert.True(t, RefundTaskQuota(context.Background(), &firstPoll, "failed"))
	assert.True(t, RefundTaskQuota(context.Background(), &stalePoll, "stale duplicate"))

	var persistedSubscription model.UserSubscription
	require.NoError(t, model.DB.First(&persistedSubscription, subscription.Id).Error)
	assert.Zero(t, persistedSubscription.AmountUsed)
	var persistedTask model.Task
	require.NoError(t, model.DB.First(&persistedTask, task.ID).Error)
	assert.Zero(t, persistedTask.Quota)
	assert.EqualValues(t, 1, persistedTask.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	var refundLogs int64
	require.NoError(t, model.DB.Model(&model.Log{}).
		Where("user_id = ? AND type = ?", fixture.user.Id, model.LogTypeRefund).
		Count(&refundLogs).Error)
	assert.EqualValues(t, 1, refundLogs)
}

func TestOrganizationSubscriptionTaskAdjustmentMetersOrganizationAndRejectsStaleDuplicate(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 0)
	subscription := model.UserSubscription{
		UserId: fixture.user.Id, AmountTotal: 1000, AmountUsed: 120,
		Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(&subscription).Error)
	seedOrganizationSubscriptionTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "organization-subscription-adjust", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			BillingSource:                   BillingSourceSubscription,
			SubscriptionId:                  subscription.Id,
			OrganizationSubscriptionMetered: true,
			TokenId:                         fixture.token.Id,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Task
	var stalePoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.ID).Error)

	RecalculateTaskQuota(context.Background(), &firstPoll, 150, "actual usage")
	RecalculateTaskQuota(context.Background(), &stalePoll, 150, "stale duplicate")

	var persistedSubscription model.UserSubscription
	require.NoError(t, model.DB.First(&persistedSubscription, subscription.Id).Error)
	assert.EqualValues(t, 150, persistedSubscription.AmountUsed)
	var persistedTask model.Task
	require.NoError(t, model.DB.First(&persistedTask, task.ID).Error)
	assert.Equal(t, 150, persistedTask.Quota)
	assert.EqualValues(t, 1, persistedTask.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Equal(t, 150, user.UsedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 150, fund.ConsumedQuota)
	assert.Equal(t, 850, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	assert.EqualValues(t, 150, channel.UsedQuota)
	var billingLogs int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ?", fixture.user.Id).Count(&billingLogs).Error)
	assert.EqualValues(t, 1, billingLogs)
}

func TestOrganizationSubscriptionTaskConsumptionLimitRollsBackAllBillingState(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 0)
	limit := int64(140)
	require.NoError(t, model.DB.Model(&model.OrganizationMemberFund{}).
		Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).
		Update("consumption_limit", limit).Error)
	subscription := model.UserSubscription{
		UserId: fixture.user.Id, AmountTotal: 1000, AmountUsed: 120,
		Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(&subscription).Error)
	seedOrganizationSubscriptionTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "organization-subscription-limit", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			BillingSource:                   BillingSourceSubscription,
			SubscriptionId:                  subscription.Id,
			OrganizationSubscriptionMetered: true,
			TokenId:                         fixture.token.Id,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)

	_, err := model.ApplyOrganizationTaskBillingMutation(model.OrganizationTaskBillingMutationParams{
		TaskId: task.ID, UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		SubscriptionId: subscription.Id, TokenId: fixture.token.Id, ChannelId: fixture.channel.Id,
		ExpectedQuota: 120, ActualQuota: 150, OperationId: "organization-subscription-limit-adjust",
	})
	require.ErrorIs(t, err, model.ErrOrganizationConsumptionLimit)

	var persistedSubscription model.UserSubscription
	require.NoError(t, model.DB.First(&persistedSubscription, subscription.Id).Error)
	assert.EqualValues(t, 120, persistedSubscription.AmountUsed)
	var persistedTask model.Task
	require.NoError(t, model.DB.First(&persistedTask, task.ID).Error)
	assert.Equal(t, 120, persistedTask.Quota)
	assert.Zero(t, persistedTask.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	require.NotNil(t, fund.ConsumptionLimit)
	assert.EqualValues(t, 140, *fund.ConsumptionLimit)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
}

func TestLegacyOrganizationSubscriptionTaskWithoutMeteringMarkerKeepsLegacySettlement(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	subscription := model.UserSubscription{
		UserId: fixture.user.Id, AmountTotal: 1000, AmountUsed: 120,
		Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(&subscription).Error)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "legacy-organization-subscription-adjust", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceSubscription, SubscriptionId: subscription.Id, TokenId: fixture.token.Id,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)

	RecalculateTaskQuota(context.Background(), &task, 150, "legacy task settlement")

	var persistedSubscription model.UserSubscription
	require.NoError(t, model.DB.First(&persistedSubscription, subscription.Id).Error)
	assert.EqualValues(t, 150, persistedSubscription.AmountUsed)
	var persistedTask model.Task
	require.NoError(t, model.DB.First(&persistedTask, task.ID).Error)
	assert.Equal(t, 150, persistedTask.Quota)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Equal(t, 150, user.UsedQuota)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 850, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	assert.EqualValues(t, 150, channel.UsedQuota)
}

func TestMigratedOrganizationTaskWalletUsesIdempotentLedgerFallback(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "legacy-organization-task-refund", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusFailure, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		LegacyOrganizationWallet: true,
		PrivateData:              model.TaskPrivateData{BillingSource: BillingSourceWallet, TokenId: fixture.token.Id},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Task
	var stalePoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.ID).Error)

	assert.True(t, RefundTaskQuota(context.Background(), &firstPoll, "failed"))
	assert.True(t, RefundTaskQuota(context.Background(), &stalePoll, "stale duplicate"))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Zero(t, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("source_type = ? AND source_id = ?", "wallet_refund", taskOrganizationAccountingID(&task, "refund:0:120")).
		Count(&ledgers).Error)
	assert.EqualValues(t, 1, ledgers)
}

func TestMigratedOrganizationTaskRefundRollsBackOnChannelFailureAndRetriesAtomically(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "legacy-organization-task-atomic-refund", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusFailure, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		LegacyOrganizationWallet: true,
		PrivateData:              model.TaskPrivateData{BillingSource: BillingSourceWallet, TokenId: fixture.token.Id},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var retry model.Task
	require.NoError(t, model.DB.First(&retry, task.ID).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_legacy_organization_task_channel_update
		BEFORE UPDATE OF used_quota ON channels
		WHEN OLD.id = 7004
		BEGIN
			SELECT RAISE(ABORT, 'forced legacy organization task channel failure');
		END;
	`).Error)

	assert.False(t, RefundTaskQuota(context.Background(), &task, "forced channel failure"))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 120, persisted.Quota)
	assert.Zero(t, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
	operationId := taskOrganizationAccountingID(&task, "refund:0:120")
	var refundLedgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("idempotency_key = ?", operationId).
		Count(&refundLedgers).Error)
	assert.Zero(t, refundLedgers)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_legacy_organization_task_channel_update").Error)
	assert.True(t, RefundTaskQuota(context.Background(), &retry, "retry after channel failure"))
	assert.True(t, RefundTaskQuota(context.Background(), &task, "stale duplicate"))

	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Zero(t, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel = loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("idempotency_key = ?", operationId).
		Count(&refundLedgers).Error)
	assert.EqualValues(t, 1, refundLedgers)
	var refundLogs int64
	require.NoError(t, model.DB.Model(&model.Log{}).
		Where("user_id = ? AND type = ?", fixture.user.Id, model.LogTypeRefund).
		Count(&refundLogs).Error)
	assert.EqualValues(t, 1, refundLogs)
}

func TestMigratedOrganizationTaskWalletAdjustmentUsesIdempotentLedgerFallback(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "legacy-organization-task-adjust", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		LegacyOrganizationWallet: true,
		PrivateData:              model.TaskPrivateData{BillingSource: BillingSourceWallet, TokenId: fixture.token.Id},
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Task
	var stalePoll model.Task
	require.NoError(t, model.DB.First(&firstPoll, task.ID).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.ID).Error)

	RecalculateTaskQuota(context.Background(), &firstPoll, 150, "actual usage")
	RecalculateTaskQuota(context.Background(), &stalePoll, 180, "stale duplicate with different actual quota")

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 150, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 50, user.Quota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 850, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	assert.EqualValues(t, 150, channel.UsedQuota)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("source_type = ? AND source_id = ?", "legacy_async_task", taskOrganizationAccountingID(&task, "adjust:0:120:150")).
		Count(&ledgers).Error)
	assert.EqualValues(t, 1, ledgers)
}

func TestNewOrganizationTaskWalletWithoutReservationFailsClosed(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Task{
		TaskID: "new-organization-task-missing-reservation", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusFailure, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{BillingSource: BillingSourceWallet, TokenId: fixture.token.Id},
	}
	require.NoError(t, model.DB.Create(&task).Error)

	assert.False(t, RefundTaskQuota(context.Background(), &task, "failed"))
	RecalculateTaskQuota(context.Background(), &task, 150, "actual usage")

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 120, persisted.Quota)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
}

func TestMigratedOrganizationMidjourneyWalletUsesIdempotentLedgerFallback(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Midjourney{
		UserId: fixture.user.Id, MjId: "legacy-organization-midjourney", Action: "IMAGINE",
		ChannelId: fixture.channel.Id, BillingChannelId: fixture.channel.Id, Quota: 120, TokenId: fixture.token.Id,
		Progress: "0%", LegacyOrganizationWallet: true,
	}
	require.NoError(t, model.DB.Create(&task).Error)
	var firstPoll model.Midjourney
	var stalePoll model.Midjourney
	require.NoError(t, model.DB.First(&firstPoll, task.Id).Error)
	require.NoError(t, model.DB.First(&stalePoll, task.Id).Error)

	assert.True(t, RefundMidjourneyQuota(context.Background(), &firstPoll, "failed"))
	assert.True(t, RefundMidjourneyQuota(context.Background(), &stalePoll, "stale duplicate"))

	var persisted model.Midjourney
	require.NoError(t, model.DB.First(&persisted, task.Id).Error)
	assert.Zero(t, persisted.Quota)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).
		Where("source_type = ?", "wallet_refund").Count(&ledgers).Error)
	assert.EqualValues(t, 1, ledgers)
}

func TestMigratedOrganizationMidjourneyWalletRejectsCorruptOrganizationSnapshotWithoutCrediting(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Midjourney{
		UserId: fixture.user.Id, MjId: "legacy-midjourney-corrupt-organization", Action: "IMAGINE",
		ChannelId: fixture.channel.Id, BillingChannelId: fixture.channel.Id, Quota: 120, TokenId: fixture.token.Id,
		Progress: "0%", LegacyOrganizationWallet: true,
	}
	require.NoError(t, model.DB.Create(&task).Error)
	require.NoError(t, model.DB.Model(&model.Midjourney{}).
		Where("id = ?", task.Id).
		Update("organization_id", fixture.organization.Id+100).Error)
	require.NoError(t, model.DB.First(&task, task.Id).Error)

	assert.False(t, RefundMidjourneyQuota(context.Background(), &task, "corrupt organization snapshot"))

	var persisted model.Midjourney
	require.NoError(t, model.DB.First(&persisted, task.Id).Error)
	assert.Equal(t, 120, persisted.Quota)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).Count(&ledgers).Error)
	assert.Zero(t, ledgers)
}

func TestNewOrganizationMidjourneyWithoutReservationFailsClosed(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 80, 0)
	seedOrganizationTaskUsage(t, fixture, 120)
	task := model.Midjourney{
		UserId: fixture.user.Id, MjId: "new-organization-midjourney-missing-reservation", Action: "IMAGINE",
		ChannelId: fixture.channel.Id, BillingChannelId: fixture.channel.Id, Quota: 120, TokenId: fixture.token.Id,
		Progress: "0%",
	}
	require.NoError(t, model.DB.Create(&task).Error)

	assert.False(t, RefundMidjourneyQuota(context.Background(), &task, "failed"))

	var persisted model.Midjourney
	require.NoError(t, model.DB.First(&persisted, task.Id).Error)
	assert.Equal(t, 120, persisted.Quota)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Zero(t, fund.RecoverableQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).Count(&ledgers).Error)
	assert.Zero(t, ledgers)
}

func TestOrganizationMidjourneyBillingStateFailureDoesNotRepeatCharge(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	relayInfo := &relaycommon.RelayInfo{
		UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		TokenId: fixture.token.Id, TokenKey: fixture.token.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
	}
	task := model.Midjourney{
		UserId: fixture.user.Id, MjId: "organization-midjourney", Action: "IMAGINE",
		ChannelId: fixture.channel.Id, Progress: "0%",
	}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, &task, 120, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	task.UpstreamAccepted = true
	require.NoError(t, task.UpdateSubmissionResult())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_organization_midjourney_token_marker
		BEFORE UPDATE OF token_id ON midjourneys
		WHEN OLD.id = 1
		BEGIN
			SELECT RAISE(ABORT, 'forced billing marker failure');
		END;
	`).Error)

	billed, err := SettleMidjourneyTaskBilling(relayInfo, &task, prepared)
	require.Error(t, err)
	assert.False(t, billed)
	var afterFailure model.Midjourney
	require.NoError(t, model.DB.First(&afterFailure, task.Id).Error)
	assert.Positive(t, afterFailure.OrganizationReservationId)
	assert.Zero(t, afterFailure.TokenId)
	assert.Equal(t, model.MidjourneyBillingStatusReserved, afterFailure.BillingStatus)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	var reservation model.OrganizationWalletReservation
	require.NoError(t, model.DB.First(&reservation, afterFailure.OrganizationReservationId).Error)
	assert.Equal(t, model.OrganizationWalletReservationReserved, reservation.Status)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_organization_midjourney_token_marker").Error)
	var retry model.Midjourney
	require.NoError(t, model.DB.First(&retry, task.Id).Error)
	billed, err = SettleMidjourneyTaskBilling(relayInfo, &retry, prepared)
	require.NoError(t, err)
	assert.True(t, billed)
	billed, err = SettleMidjourneyTaskBilling(relayInfo, &retry, prepared)
	require.NoError(t, err)
	assert.True(t, billed)

	user, fund, token, channel = loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
	var persisted model.Midjourney
	require.NoError(t, model.DB.First(&persisted, task.Id).Error)
	assert.Equal(t, fixture.token.Id, persisted.TokenId)
	var reservationCount int64
	require.NoError(t, model.DB.Model(&model.OrganizationWalletReservation{}).Count(&reservationCount).Error)
	assert.EqualValues(t, 1, reservationCount)
}

func TestOrganizationMidjourneyTokenSettlementSupportsUnlimitedRedisBatchMode(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	useTaskBillingMiniRedis(t)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
	})
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
		Update("unlimited_quota", true).Error)
	tokenCacheKey := seedTaskBillingTokenCache(t, fixture.token.Id, fixture.token.Key, 1000)

	relayInfo := &relaycommon.RelayInfo{
		UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		TokenId: fixture.token.Id, TokenKey: fixture.token.Key, TokenUnlimited: true,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
	}
	task := model.Midjourney{
		UserId: fixture.user.Id, MjId: "organization-midjourney-unlimited", Action: "IMAGINE",
		ChannelId: fixture.channel.Id, Progress: "0%",
	}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, &task, 120, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	task.UpstreamAccepted = true
	require.NoError(t, task.UpdateSubmissionResult())

	billed, err := SettleMidjourneyTaskBilling(relayInfo, &task, prepared)
	require.NoError(t, err)
	assert.True(t, billed)
	applied, err := task.SettleTokenQuota(fixture.token.Id, fixture.token.Key, 120)
	require.NoError(t, err)
	assert.False(t, applied)

	var persistedToken model.Token
	require.NoError(t, model.DB.First(&persistedToken, fixture.token.Id).Error)
	assert.True(t, persistedToken.UnlimitedQuota)
	assert.Equal(t, 880, persistedToken.RemainQuota)
	assert.Equal(t, 120, persistedToken.UsedQuota)
	cachedRemainQuota, cachedUsedQuota := getTaskBillingTokenCacheQuota(t, tokenCacheKey)
	assert.Equal(t, 880, cachedRemainQuota)
	assert.Equal(t, 120, cachedUsedQuota)
}
