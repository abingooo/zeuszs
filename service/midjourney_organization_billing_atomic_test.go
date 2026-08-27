package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const midjourneyAtomicTestQuota = 120

func dispatchOrganizationBillingLogs(t *testing.T) {
	t.Helper()
	result, err := model.DispatchBillingLogOutbox(context.Background(), "service-test", 100, time.Second, 0)
	require.NoError(t, err)
	assert.Equal(t, result.Claimed, result.Delivered)
}

func createPreparedOrganizationMidjourney(t *testing.T, fixture organizationTaskBillingFixture) (*relaycommon.RelayInfo, *model.Midjourney) {
	t.Helper()
	relayInfo := &relaycommon.RelayInfo{
		UserId:         fixture.user.Id,
		OrganizationId: fixture.organization.Id,
		TokenId:        fixture.token.Id,
		TokenKey:       fixture.token.Key,
		ChannelMeta:    &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
	}
	task := &model.Midjourney{
		UserId:     fixture.user.Id,
		Action:     "IMAGINE",
		ChannelId:  fixture.channel.Id,
		SubmitTime: 1000,
		Status:     model.MidjourneyStatusSubmitting,
		Progress:   "0%",
	}
	prepared, err := CreateMidjourneyTaskBilling(relayInfo, task, midjourneyAtomicTestQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.Positive(t, task.Id)
	require.Positive(t, task.OrganizationReservationId)
	require.Equal(t, model.MidjourneyBillingStatusReserved, task.BillingStatus)
	return relayInfo, task
}

func acceptOrganizationMidjourney(t *testing.T, task *model.Midjourney, upstreamID string) {
	t.Helper()
	task.MjId = upstreamID
	task.Status = "IN_PROGRESS"
	task.UpstreamAccepted = true
	require.NoError(t, task.UpdateSubmissionResult())
}

func loadOrganizationMidjourney(t *testing.T, id int) model.Midjourney {
	t.Helper()
	var task model.Midjourney
	require.NoError(t, model.DB.First(&task, id).Error)
	return task
}

func loadOrganizationMidjourneyReservation(t *testing.T, id int64) model.OrganizationWalletReservation {
	t.Helper()
	var reservation model.OrganizationWalletReservation
	require.NoError(t, model.DB.First(&reservation, id).Error)
	return reservation
}

func assertOrganizationMidjourneyPrepareRolledBack(t *testing.T, fixture organizationTaskBillingFixture) {
	t.Helper()
	var taskCount, reservationCount, ledgerCount, operationCount int64
	require.NoError(t, model.DB.Model(&model.Midjourney{}).Count(&taskCount).Error)
	require.NoError(t, model.DB.Model(&model.OrganizationWalletReservation{}).Count(&reservationCount).Error)
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaLedger{}).Count(&ledgerCount).Error)
	require.NoError(t, model.DB.Model(&model.OrganizationQuotaOperation{}).Count(&operationCount).Error)
	assert.Zero(t, taskCount)
	assert.Zero(t, reservationCount)
	assert.Zero(t, ledgerCount)
	assert.Zero(t, operationCount)

	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
}

func TestOrganizationMidjourneyPrepareFailuresRollBackEveryMutation(t *testing.T) {
	tests := []struct {
		name    string
		trigger string
	}{
		{
			name: "token reservation",
			trigger: `
				CREATE TRIGGER fail_midjourney_prepare_token
				BEFORE UPDATE OF remain_quota ON tokens
				WHEN OLD.id = 7003
				BEGIN
					SELECT RAISE(ABORT, 'forced Midjourney token reservation failure');
				END;
			`,
		},
		{
			name: "reservation binding",
			trigger: `
				CREATE TRIGGER fail_midjourney_prepare_binding
				BEFORE UPDATE OF organization_reservation_id ON midjourneys
				BEGIN
					SELECT RAISE(ABORT, 'forced Midjourney reservation binding failure');
				END;
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setupOrganizationTaskBillingTest(t, 200, 100)
			require.NoError(t, model.DB.Exec(test.trigger).Error)
			relayInfo := &relaycommon.RelayInfo{
				UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
				TokenId: fixture.token.Id, TokenKey: fixture.token.Key,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
			}
			task := &model.Midjourney{
				UserId: fixture.user.Id, Action: "IMAGINE", ChannelId: fixture.channel.Id,
				SubmitTime: 1000, Status: model.MidjourneyStatusSubmitting, Progress: "0%",
			}

			prepared, err := CreateMidjourneyTaskBilling(relayInfo, task, midjourneyAtomicTestQuota, true)
			require.Error(t, err)
			assert.False(t, prepared)
			assertOrganizationMidjourneyPrepareRolledBack(t, fixture)
		})
	}
}

func TestOrganizationMidjourneyInsufficientTokenFailsBeforeCreatingTask(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
		Update("remain_quota", midjourneyAtomicTestQuota-1).Error)
	relayInfo := &relaycommon.RelayInfo{
		UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		TokenId: fixture.token.Id, TokenKey: fixture.token.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
	}
	task := &model.Midjourney{
		UserId: fixture.user.Id, Action: "IMAGINE", ChannelId: fixture.channel.Id,
		SubmitTime: 1000, Status: model.MidjourneyStatusSubmitting, Progress: "0%",
	}

	prepared, err := CreateMidjourneyTaskBilling(relayInfo, task, midjourneyAtomicTestQuota, true)
	require.Error(t, err)
	assert.False(t, prepared)
	var taskCount, reservationCount int64
	require.NoError(t, model.DB.Model(&model.Midjourney{}).Count(&taskCount).Error)
	require.NoError(t, model.DB.Model(&model.OrganizationWalletReservation{}).Count(&reservationCount).Error)
	assert.Zero(t, taskCount)
	assert.Zero(t, reservationCount)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, midjourneyAtomicTestQuota-1, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
}

func TestOrganizationMidjourneyLimitedTokenFailsClosedWhenRedisIsAuthoritative(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	useTaskBillingMiniRedis(t)
	tokenCacheKey := seedTaskBillingTokenCache(t, fixture.token.Id, fixture.token.Key, 40)
	relayInfo := &relaycommon.RelayInfo{
		UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		TokenId: fixture.token.Id, TokenKey: fixture.token.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
	}
	task := &model.Midjourney{
		UserId: fixture.user.Id, Action: "IMAGINE", ChannelId: fixture.channel.Id,
		SubmitTime: 1000, Status: model.MidjourneyStatusSubmitting, Progress: "0%",
	}

	prepared, err := CreateMidjourneyTaskBilling(relayInfo, task, midjourneyAtomicTestQuota, true)
	require.ErrorIs(t, err, model.ErrMidjourneyRedisTokenReservationUnsupported)
	assert.False(t, prepared)
	var taskCount, reservationCount int64
	require.NoError(t, model.DB.Model(&model.Midjourney{}).Count(&taskCount).Error)
	require.NoError(t, model.DB.Model(&model.OrganizationWalletReservation{}).Count(&reservationCount).Error)
	assert.Zero(t, taskCount)
	assert.Zero(t, reservationCount)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	cachedRemain, cachedUsed := getTaskBillingTokenCacheQuota(t, tokenCacheKey)
	assert.Equal(t, 40, cachedRemain)
	assert.Zero(t, cachedUsed)
}

func TestOrganizationMidjourneySettlementFailuresRollBackAndRetryOnce(t *testing.T) {
	tests := []struct {
		name        string
		triggerName string
		trigger     string
	}{
		{
			name:        "user usage",
			triggerName: "fail_midjourney_settle_user_usage",
			trigger: `
				CREATE TRIGGER fail_midjourney_settle_user_usage
				BEFORE UPDATE OF used_quota ON users
				WHEN OLD.id = 7002 AND NEW.used_quota > OLD.used_quota
				BEGIN
					SELECT RAISE(ABORT, 'forced Midjourney user usage failure');
				END;
			`,
		},
		{
			name:        "channel usage",
			triggerName: "fail_midjourney_settle_channel_usage",
			trigger: `
				CREATE TRIGGER fail_midjourney_settle_channel_usage
				BEFORE UPDATE OF used_quota ON channels
				WHEN OLD.id = 7004 AND NEW.used_quota > OLD.used_quota
				BEGIN
					SELECT RAISE(ABORT, 'forced Midjourney channel usage failure');
				END;
			`,
		},
		{
			name:        "billing log outbox",
			triggerName: "fail_midjourney_settle_outbox",
			trigger: `
				CREATE TRIGGER fail_midjourney_settle_outbox
				BEFORE INSERT ON billing_log_outboxes
				BEGIN
					SELECT RAISE(ABORT, 'forced Midjourney billing log outbox failure');
				END;
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setupOrganizationTaskBillingTest(t, 200, 100)
			_, task := createPreparedOrganizationMidjourney(t, fixture)
			acceptOrganizationMidjourney(t, task, "midjourney-settle-"+test.name)
			require.NoError(t, model.DB.Exec(test.trigger).Error)

			result, err := task.SettleOrganizationBilling()
			require.Error(t, err)
			assert.False(t, result.Applied)
			assert.False(t, result.Settled)
			persisted := loadOrganizationMidjourney(t, task.Id)
			assert.Equal(t, model.MidjourneyBillingStatusReserved, persisted.BillingStatus)
			assert.Zero(t, persisted.TokenId)
			reservation := loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
			assert.Equal(t, model.OrganizationWalletReservationReserved, reservation.Status)
			user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
			assert.Equal(t, 80, user.Quota)
			assert.Zero(t, user.UsedQuota)
			assert.Zero(t, user.RequestCount)
			assert.Zero(t, fund.RecoverableQuota)
			assert.EqualValues(t, 120, fund.ConsumedQuota)
			assert.Equal(t, 880, token.RemainQuota)
			assert.Equal(t, 120, token.UsedQuota)
			assert.Zero(t, channel.UsedQuota)

			require.NoError(t, model.DB.Exec("DROP TRIGGER "+test.triggerName).Error)
			persisted = loadOrganizationMidjourney(t, task.Id)
			result, err = persisted.SettleOrganizationBilling()
			require.NoError(t, err)
			assert.True(t, result.Applied)
			assert.True(t, result.Settled)
			result, err = persisted.SettleOrganizationBilling()
			require.NoError(t, err)
			assert.False(t, result.Applied)
			assert.True(t, result.Settled)
			persisted = loadOrganizationMidjourney(t, task.Id)
			assert.Equal(t, model.MidjourneyBillingStatusSettled, persisted.BillingStatus)
			assert.Equal(t, fixture.token.Id, persisted.TokenId)
			reservation = loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
			assert.Equal(t, model.OrganizationWalletReservationSettled, reservation.Status)
			user, fund, token, channel = loadOrganizationTaskBillingBalances(t, fixture)
			assert.Equal(t, 80, user.Quota)
			assert.Equal(t, 120, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			assert.Zero(t, fund.RecoverableQuota)
			assert.EqualValues(t, 120, fund.ConsumedQuota)
			assert.Equal(t, 880, token.RemainQuota)
			assert.Equal(t, 120, token.UsedQuota)
			assert.EqualValues(t, 120, channel.UsedQuota)
			var consumeOutboxCount int64
			require.NoError(t, model.DB.Model(&model.BillingLogOutbox{}).
				Where("event_key = ?", fmt.Sprintf("midjourney:%d:consume", task.Id)).
				Count(&consumeOutboxCount).Error)
			assert.EqualValues(t, 1, consumeOutboxCount)
		})
	}
}

func TestOrganizationMidjourneySettlementSurvivesDeletedBillingChannel(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-deleted-channel-settle")
	require.NoError(t, model.DB.Delete(&model.Channel{}, fixture.channel.Id).Error)

	result, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.True(t, result.Settled)
	persisted := loadOrganizationMidjourney(t, task.Id)
	assert.Equal(t, model.MidjourneyBillingStatusSettled, persisted.BillingStatus)
	assert.Equal(t, fixture.token.Id, persisted.TokenId)
	reservation := loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
	assert.Equal(t, model.OrganizationWalletReservationSettled, reservation.Status)
	var user model.User
	var fund model.OrganizationMemberFund
	var token model.Token
	require.NoError(t, model.DB.First(&user, fixture.user.Id).Error)
	require.NoError(t, model.DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	require.NoError(t, model.DB.First(&token, fixture.token.Id).Error)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	var channelCount int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.channel.Id).Count(&channelCount).Error)
	assert.Zero(t, channelCount)
	var consumeEvent model.BillingLogOutbox
	require.NoError(t, model.DB.Where("event_key = ?", fmt.Sprintf("midjourney:%d:consume", task.Id)).First(&consumeEvent).Error)
	var consumePayload model.BillingLogOutboxPayload
	require.NoError(t, common.UnmarshalJsonStr(consumeEvent.Payload, &consumePayload))
	assert.Equal(t, fixture.organization.Name, consumePayload.Organization.Name)
	assert.Equal(t, fixture.user.Username, consumePayload.User.Name)
	assert.Equal(t, fixture.token.Name, consumePayload.Token.Name)
	assert.Equal(t, fixture.channel.Id, consumePayload.Channel.Id)
	assert.Empty(t, consumePayload.Channel.Name)
	dispatchOrganizationBillingLogs(t)
	var consumeLogCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).
		Where("billing_event_id = ?", fmt.Sprintf("midjourney:%d:consume", task.Id)).
		Count(&consumeLogCount).Error)
	assert.EqualValues(t, 1, consumeLogCount)
}

func TestOrganizationMidjourneyRefundSurvivesDeletedBillingChannel(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-deleted-channel-refund")
	result, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, model.DB.Delete(&model.Channel{}, fixture.channel.Id).Error)

	persisted := loadOrganizationMidjourney(t, task.Id)
	refunded, err := persisted.RefundOrganizationBilling("billing channel deleted", true)
	require.NoError(t, err)
	assert.True(t, refunded)
	persisted = loadOrganizationMidjourney(t, task.Id)
	assert.Equal(t, model.MidjourneyBillingStatusRefunded, persisted.BillingStatus)
	assert.Equal(t, "FAILURE", persisted.Status)
	assert.Zero(t, persisted.Quota)
	reservation := loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
	assert.Equal(t, model.OrganizationWalletReservationRefunded, reservation.Status)
	var user model.User
	var fund model.OrganizationMemberFund
	var token model.Token
	require.NoError(t, model.DB.First(&user, fixture.user.Id).Error)
	require.NoError(t, model.DB.Where("organization_id = ? AND user_id = ?", fixture.organization.Id, fixture.user.Id).First(&fund).Error)
	require.NoError(t, model.DB.First(&token, fixture.token.Id).Error)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	var channelCount int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.channel.Id).Count(&channelCount).Error)
	assert.Zero(t, channelCount)
	var refundEvent model.BillingLogOutbox
	require.NoError(t, model.DB.Where("event_key = ?", fmt.Sprintf("midjourney:%d:refund", task.Id)).First(&refundEvent).Error)
	var refundPayload model.BillingLogOutboxPayload
	require.NoError(t, common.UnmarshalJsonStr(refundEvent.Payload, &refundPayload))
	assert.Equal(t, fixture.channel.Id, refundPayload.Channel.Id)
	assert.Empty(t, refundPayload.Channel.Name)
	assert.Equal(t, model.OrganizationWalletReservationRefunded, refundPayload.Reservation.Status)
	dispatchOrganizationBillingLogs(t)
	var refundLogCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).
		Where("billing_event_id = ?", fmt.Sprintf("midjourney:%d:refund", task.Id)).
		Count(&refundLogCount).Error)
	assert.EqualValues(t, 1, refundLogCount)
}

func TestOrganizationMidjourneyOutboxDeliveryUsesImmutableSnapshotsAfterResourceDeletion(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-delayed-log-snapshot")
	result, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	require.True(t, result.Applied)

	var event model.BillingLogOutbox
	require.NoError(t, model.DB.Where("event_key = ?", fmt.Sprintf("midjourney:%d:consume", task.Id)).First(&event).Error)
	var payload model.BillingLogOutboxPayload
	require.NoError(t, common.UnmarshalJsonStr(event.Payload, &payload))
	assert.Equal(t, fixture.user.Username, payload.User.Name)
	assert.Equal(t, fixture.token.Name, payload.Token.Name)
	assert.Equal(t, fixture.channel.Name, payload.Channel.Name)

	require.NoError(t, model.DB.Model(&model.Midjourney{}).Where("id = ?", task.Id).
		Updates(map[string]interface{}{"status": "SUCCESS", "progress": "100%"}).Error)
	require.NoError(t, model.DB.Exec("DELETE FROM tokens WHERE id = ?", fixture.token.Id).Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels WHERE id = ?", fixture.channel.Id).Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users WHERE id = ?", fixture.user.Id).Error)

	dispatchOrganizationBillingLogs(t)
	var delivered model.Log
	require.NoError(t, model.LOG_DB.Where("billing_event_id = ?", event.EventKey).First(&delivered).Error)
	assert.Equal(t, fixture.user.Username, delivered.Username)
	assert.Equal(t, fixture.token.Name, delivered.TokenName)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(delivered.Other, &other))
	billingEvent, ok := other["billing_event"].(map[string]interface{})
	require.True(t, ok)
	channel, ok := billingEvent["channel"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, fixture.channel.Name, channel["name"])
}

func TestOrganizationMidjourneyTokenModeChangesDoNotRewriteBillingHistory(t *testing.T) {
	tests := []struct {
		name            string
		unlimitedBefore bool
		unlimitedAfter  bool
	}{
		{name: "limited becomes unlimited", unlimitedBefore: false, unlimitedAfter: true},
		{name: "unlimited becomes limited", unlimitedBefore: true, unlimitedAfter: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setupOrganizationTaskBillingTest(t, 200, 100)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
				Update("unlimited_quota", test.unlimitedBefore).Error)
			_, task := createPreparedOrganizationMidjourney(t, fixture)
			acceptOrganizationMidjourney(t, task, "midjourney-token-mode-"+test.name)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
				Update("unlimited_quota", test.unlimitedAfter).Error)

			settled, err := task.SettleOrganizationBilling()
			require.NoError(t, err)
			assert.True(t, settled.Applied)
			persisted := loadOrganizationMidjourney(t, task.Id)
			assert.Equal(t, fixture.token.Id, persisted.TokenId)
			refunded, err := persisted.RefundOrganizationBilling("upstream failure", true)
			require.NoError(t, err)
			assert.True(t, refunded)

			persisted = loadOrganizationMidjourney(t, task.Id)
			assert.Equal(t, model.MidjourneyBillingStatusRefunded, persisted.BillingStatus)
			assert.Equal(t, fixture.token.Id, persisted.TokenId)
			assert.Zero(t, persisted.Quota)
			user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
			assert.Equal(t, 200, user.Quota)
			assert.Zero(t, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			assert.EqualValues(t, 100, fund.RecoverableQuota)
			assert.Zero(t, fund.ConsumedQuota)
			assert.Equal(t, 1000, token.RemainQuota)
			assert.Zero(t, token.UsedQuota)
			assert.Equal(t, test.unlimitedAfter, token.UnlimitedQuota)
			assert.Zero(t, channel.UsedQuota)
		})
	}
}

func TestRecoverMidjourneyBillingSettlesAcceptedTaskIntoPolling(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-recovery-accepted")

	summary := RecoverMidjourneyBilling(context.Background(), 2000, 10)
	assert.Equal(t, 1, summary.Settled)
	assert.Zero(t, summary.Refunded)
	assert.Zero(t, summary.Failed)
	persisted := loadOrganizationMidjourney(t, task.Id)
	assert.Equal(t, model.MidjourneyBillingStatusSettled, persisted.BillingStatus)
	assert.Equal(t, fixture.token.Id, persisted.TokenId)
	unfinished := model.GetAllUnFinishTasks()
	require.Len(t, unfinished, 1)
	assert.Equal(t, task.Id, unfinished[0].Id)
	dispatchOrganizationBillingLogs(t)
	var recoveryLogs []model.Log
	require.NoError(t, model.LOG_DB.
		Where("user_id = ? AND type = ?", fixture.user.Id, model.LogTypeConsume).
		Find(&recoveryLogs).Error)
	require.Len(t, recoveryLogs, 1)
	assert.Equal(t, fixture.organization.Id, recoveryLogs[0].OrganizationId)
	assert.Equal(t, midjourneyAtomicTestQuota, recoveryLogs[0].Quota)
	assert.Equal(t, fixture.channel.Id, recoveryLogs[0].ChannelId)
	assert.Equal(t, fixture.token.Id, recoveryLogs[0].TokenId)
	assert.Equal(t, "Midjourney task consumption", recoveryLogs[0].Content)
	logOther, err := common.StrToMap(recoveryLogs[0].Other)
	require.NoError(t, err)
	assert.Equal(t, task.MjId, logOther["task_id"])
	assert.EqualValues(t, task.Id, logOther["local_task_id"])

	second := RecoverMidjourneyBilling(context.Background(), 2000, 10)
	assert.Zero(t, second.Settled)
	assert.Zero(t, second.Refunded)
	assert.Zero(t, second.Failed)
	var recoveryLogCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).
		Where("user_id = ? AND type = ?", fixture.user.Id, model.LogTypeConsume).
		Count(&recoveryLogCount).Error)
	assert.EqualValues(t, 1, recoveryLogCount)
	user, _, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
}

func TestRecoverMidjourneyBillingRefundsRejectedAndMissingIDs(t *testing.T) {
	tests := []struct {
		name       string
		upstreamID string
	}{
		{name: "rejected response with task id", upstreamID: "midjourney-rejected-with-id"},
		{name: "missing task id", upstreamID: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setupOrganizationTaskBillingTest(t, 200, 100)
			_, task := createPreparedOrganizationMidjourney(t, fixture)
			if test.upstreamID != "" {
				task.MjId = test.upstreamID
				task.Status = "FAILURE"
				task.Progress = "100%"
				task.FailReason = "rejected"
				task.UpstreamAccepted = false
				require.NoError(t, task.UpdateSubmissionResult())
				assert.True(t, model.HasUnfinishedMidjourneyTasks())
			}

			summary := RecoverMidjourneyBilling(context.Background(), 2000, 10)
			assert.Zero(t, summary.Settled)
			assert.Equal(t, 1, summary.Refunded)
			assert.Zero(t, summary.Failed)
			persisted := loadOrganizationMidjourney(t, task.Id)
			assert.Equal(t, model.MidjourneyBillingStatusRefunded, persisted.BillingStatus)
			assert.Equal(t, "FAILURE", persisted.Status)
			assert.Equal(t, "100%", persisted.Progress)
			assert.Zero(t, persisted.Quota)
			assert.Zero(t, persisted.TokenId)
			reservation := loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
			assert.Equal(t, model.OrganizationWalletReservationRefunded, reservation.Status)
			user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
			assert.Equal(t, 200, user.Quota)
			assert.Zero(t, user.UsedQuota)
			assert.Zero(t, user.RequestCount)
			assert.EqualValues(t, 100, fund.RecoverableQuota)
			assert.Zero(t, fund.ConsumedQuota)
			assert.Equal(t, 1000, token.RemainQuota)
			assert.Zero(t, token.UsedQuota)
			assert.Zero(t, channel.UsedQuota)

			second := RecoverMidjourneyBilling(context.Background(), 2000, 10)
			assert.Zero(t, second.Settled)
			assert.Zero(t, second.Refunded)
			assert.Zero(t, second.Failed)
		})
	}
}

func TestOrganizationMidjourneyRefundFailureRollsBackAndRetriesOnce(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-refund-atomic")
	settled, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	require.True(t, settled.Applied)
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`
		CREATE TRIGGER fail_midjourney_refund_outbox
		BEFORE INSERT ON billing_log_outboxes
		WHEN NEW.event_key = 'midjourney:%d:refund'
		BEGIN
			SELECT RAISE(ABORT, 'forced Midjourney refund outbox failure');
		END;
	`, task.Id)).Error)

	persisted := loadOrganizationMidjourney(t, task.Id)
	refunded, err := persisted.RefundOrganizationBilling("upstream failure", true)
	require.Error(t, err)
	assert.False(t, refunded)
	persisted = loadOrganizationMidjourney(t, task.Id)
	assert.Equal(t, model.MidjourneyBillingStatusSettled, persisted.BillingStatus)
	assert.Equal(t, "IN_PROGRESS", persisted.Status)
	assert.Equal(t, midjourneyAtomicTestQuota, persisted.Quota)
	assert.Equal(t, fixture.token.Id, persisted.TokenId)
	reservation := loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
	assert.Equal(t, model.OrganizationWalletReservationSettled, reservation.Status)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_midjourney_refund_outbox").Error)
	persisted = loadOrganizationMidjourney(t, task.Id)
	refunded, err = persisted.RefundOrganizationBilling("upstream failure", true)
	require.NoError(t, err)
	assert.True(t, refunded)
	refunded, err = persisted.RefundOrganizationBilling("duplicate", true)
	require.NoError(t, err)
	assert.False(t, refunded)
	persisted = loadOrganizationMidjourney(t, task.Id)
	assert.Equal(t, model.MidjourneyBillingStatusRefunded, persisted.BillingStatus)
	assert.Equal(t, "FAILURE", persisted.Status)
	assert.Equal(t, "100%", persisted.Progress)
	assert.Zero(t, persisted.Quota)
	reservation = loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
	assert.Equal(t, model.OrganizationWalletReservationRefunded, reservation.Status)
	user, fund, token, channel = loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	var refundOutboxCount int64
	require.NoError(t, model.DB.Model(&model.BillingLogOutbox{}).
		Where("event_key = ?", fmt.Sprintf("midjourney:%d:refund", task.Id)).
		Count(&refundOutboxCount).Error)
	assert.EqualValues(t, 1, refundOutboxCount)
}

func TestOrganizationMidjourneyStaleFailureCannotRefundSuccessfulTask(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-stale-failure")
	settled, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	require.True(t, settled.Applied)
	stale := loadOrganizationMidjourney(t, task.Id)
	require.NoError(t, model.DB.Model(&model.Midjourney{}).Where("id = ? AND status = ?", task.Id, "IN_PROGRESS").
		Updates(map[string]interface{}{"status": "SUCCESS", "progress": "100%"}).Error)

	refunded, err := stale.FailAndRefundOrganizationBilling("IN_PROGRESS", "stale failure")
	require.NoError(t, err)
	assert.False(t, refunded)
	persisted := loadOrganizationMidjourney(t, task.Id)
	assert.Equal(t, "SUCCESS", persisted.Status)
	assert.Equal(t, "100%", persisted.Progress)
	assert.Equal(t, model.MidjourneyBillingStatusSettled, persisted.BillingStatus)
	assert.Equal(t, midjourneyAtomicTestQuota, persisted.Quota)
	reservation := loadOrganizationMidjourneyReservation(t, persisted.OrganizationReservationId)
	assert.Equal(t, model.OrganizationWalletReservationSettled, reservation.Status)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Zero(t, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
}

func TestMidjourneySubmissionResultVerifiesZeroChangedRows(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	relayInfo := &relaycommon.RelayInfo{
		UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: fixture.channel.Id},
	}
	task := &model.Midjourney{
		UserId: fixture.user.Id, Action: "UPLOAD", ChannelId: fixture.channel.Id,
		SubmitTime: 1000, Status: model.MidjourneyStatusSubmitting, Progress: "0%",
	}
	prepared, err := CreateMidjourneyTaskBilling(relayInfo, task, 0, false)
	require.NoError(t, err)
	assert.False(t, prepared)
	task.Code = 4
	task.Status = "FAILURE"
	task.Progress = "100%"
	task.FailReason = "rejected"
	task.Description = "rejected"
	require.NoError(t, task.UpdateSubmissionResult())
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`
		CREATE TRIGGER ignore_midjourney_submission_noop
		BEFORE UPDATE ON midjourneys
		WHEN OLD.id = %d
		BEGIN
			SELECT RAISE(IGNORE);
		END;
	`, task.Id)).Error)

	assert.True(t, CancelPreparedMidjourneyTaskBilling(context.Background(), task, "rejected"))
	task.Description = "concurrent overwrite"
	assert.ErrorIs(t, task.UpdateSubmissionResult(), model.ErrOrganizationReservationState)
}
