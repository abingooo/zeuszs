package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationMidjourneyBillingSaturatesPersistedCounters(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 500, 300)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
		Update("used_quota", common.MaxQuota-10).Error)

	_, task := createPreparedOrganizationMidjourney(t, fixture)
	require.NotNil(t, task.QuotaClamp)
	assert.Equal(t, common.QuotaClampOverflow, task.QuotaClamp.Kind)
	acceptOrganizationMidjourney(t, task, "midjourney-counter-saturation")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fixture.user.Id).
		Updates(map[string]interface{}{
			"used_quota":    common.MaxQuota - 10,
			"request_count": common.MaxQuota,
		}).Error)

	result, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.True(t, result.Settled)
	require.NotNil(t, result.QuotaClamp)
	assert.Equal(t, common.QuotaClampOverflow, result.QuotaClamp.Kind)

	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 380, user.Quota)
	assert.Equal(t, common.MaxQuota, user.UsedQuota)
	assert.Equal(t, common.MaxQuota, user.RequestCount)
	assert.EqualValues(t, 180, fund.RecoverableQuota)
	assert.EqualValues(t, 120, fund.ConsumedQuota)
	assert.Equal(t, 880, token.RemainQuota)
	assert.Equal(t, common.MaxQuota, token.UsedQuota)
	assert.EqualValues(t, 120, channel.UsedQuota)
	assert.Equal(t, model.MidjourneyBillingStatusSettled, loadOrganizationMidjourney(t, task.Id).BillingStatus)
}

func TestOrganizationMidjourneyRefundFloorsCorruptCountersAtZero(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	_, task := createPreparedOrganizationMidjourney(t, fixture)
	acceptOrganizationMidjourney(t, task, "midjourney-counter-underflow")
	result, err := task.SettleOrganizationBilling()
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fixture.user.Id).
		Update("used_quota", 5).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
		Update("used_quota", 5).Error)

	persisted := loadOrganizationMidjourney(t, task.Id)
	assert.True(t, RefundMidjourneyQuota(context.Background(), &persisted, "counter repair refund"))
	require.NotNil(t, persisted.QuotaClamp)
	assert.Equal(t, common.QuotaClampUnderflow, persisted.QuotaClamp.Kind)
	assert.Zero(t, persisted.QuotaClamp.Clamped)

	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 200, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.EqualValues(t, 100, fund.RecoverableQuota)
	assert.Zero(t, fund.ConsumedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, channel.UsedQuota)
	assert.Equal(t, model.MidjourneyBillingStatusRefunded, loadOrganizationMidjourney(t, task.Id).BillingStatus)

	dispatchOrganizationBillingLogs(t)
	var refundLog model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Order("id DESC").First(&refundLog).Error)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(refundLog.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	quotaSaturation, ok := adminInfo["quota_saturation"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, string(common.QuotaClampUnderflow), quotaSaturation["kind"])
	assert.EqualValues(t, 0, quotaSaturation["clamped"])
}

func TestOrganizationTaskAdjustmentSaturatesPersistedCounters(t *testing.T) {
	fixture := setupOrganizationTaskBillingTest(t, 200, 100)
	reservation := reserveSettledOrganizationTaskWallet(t, fixture, 120, "organization-task-counter-saturation")
	seedOrganizationTaskUsage(t, fixture, 120)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fixture.user.Id).
		Update("used_quota", common.MaxQuota-5).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
		Update("used_quota", common.MaxQuota-5).Error)
	task := model.Task{
		TaskID: "organization-task-counter-saturation", UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		Quota: 120, Status: model.TaskStatusSuccess, Group: "default", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			OrganizationReservationId: reservation.Id,
			TokenId:                   fixture.token.Id,
			BillingSource:             BillingSourceWallet,
		},
	}
	require.NoError(t, model.DB.Create(&task).Error)

	result, err := model.ApplyOrganizationTaskBillingMutation(model.OrganizationTaskBillingMutationParams{
		TaskId: task.ID, UserId: fixture.user.Id, OrganizationId: fixture.organization.Id,
		OrganizationReservationId: reservation.Id, TokenId: fixture.token.Id, ChannelId: fixture.channel.Id,
		ExpectedQuota: 120, ActualQuota: 130, OperationId: "task-counter-saturation-adjust",
	})
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, 10, result.QuotaDelta)
	require.NotNil(t, result.QuotaClamp)
	assert.Equal(t, common.QuotaClampOverflow, result.QuotaClamp.Kind)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 130, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
	user, fund, token, channel := loadOrganizationTaskBillingBalances(t, fixture)
	assert.Equal(t, 70, user.Quota)
	assert.Equal(t, common.MaxQuota, user.UsedQuota)
	assert.EqualValues(t, 130, fund.ConsumedQuota)
	assert.Equal(t, 870, token.RemainQuota)
	assert.Equal(t, common.MaxQuota, token.UsedQuota)
	assert.EqualValues(t, 130, channel.UsedQuota)
}
