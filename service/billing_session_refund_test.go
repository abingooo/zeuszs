package service

import (
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type billingRefundTestFunding struct {
	calls    atomic.Int32
	refundFn func(call int32) error
}

func (*billingRefundTestFunding) Source() string       { return BillingSourceWallet }
func (*billingRefundTestFunding) PreConsume(int) error { return nil }
func (*billingRefundTestFunding) Settle(int) error     { return nil }

func (f *billingRefundTestFunding) Refund() error {
	call := f.calls.Add(1)
	if f.refundFn == nil {
		return nil
	}
	return f.refundFn(call)
}

func billingRefundTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func requireBillingRefundState(t *testing.T, session *BillingSession, expected billingRefundState) {
	t.Helper()
	require.Eventually(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return session.refundState == expected
	}, time.Second, 5*time.Millisecond)
}

func loadBillingRefundToken(t *testing.T, tokenId int) model.Token {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenId).Error)
	return token
}

func TestBillingSessionRefundRetriesOnlyTokenAfterFundingSucceeded(t *testing.T) {
	truncate(t)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousRedisEnabled := common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.RedisEnabled = previousRedisEnabled
		model.DB.Exec("DROP TRIGGER IF EXISTS fail_billing_session_token_refund")
	})

	const userId, tokenId, consumed = 8101, 8102, 100
	seedUser(t, userId, 900)
	seedToken(t, tokenId, userId, "billing-session-refund-token", 900)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenId).Update("used_quota", consumed).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_billing_session_token_refund
		BEFORE UPDATE OF remain_quota ON tokens
		WHEN OLD.id = 8102
		BEGIN
			SELECT RAISE(ABORT, 'forced billing session token refund failure');
		END;
	`).Error)

	funding := &WalletFunding{userId: userId, consumed: consumed}
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId: userId, TokenId: tokenId, TokenKey: "billing-session-refund-token",
		},
		funding:          funding,
		preConsumedQuota: consumed,
		tokenConsumed:    consumed,
	}

	session.Refund(billingRefundTestContext())
	requireBillingRefundState(t, session, billingRefundPending)
	assert.True(t, session.NeedsRefund())
	session.mu.Lock()
	assert.True(t, session.fundingRefunded)
	assert.False(t, session.tokenRefunded)
	session.mu.Unlock()
	userQuota, err := model.GetUserQuota(userId, false)
	require.NoError(t, err)
	assert.Equal(t, 1000, userQuota)
	token := loadBillingRefundToken(t, tokenId)
	assert.Equal(t, 900, token.RemainQuota)
	assert.Equal(t, consumed, token.UsedQuota)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_billing_session_token_refund").Error)
	session.Refund(billingRefundTestContext())
	requireBillingRefundState(t, session, billingRefundCompleted)
	assert.False(t, session.NeedsRefund())
	userQuota, err = model.GetUserQuota(userId, false)
	require.NoError(t, err)
	assert.Equal(t, 1000, userQuota, "successful funding refund must not run twice")
	token = loadBillingRefundToken(t, tokenId)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)

	session.Refund(billingRefundTestContext())
	userQuota, err = model.GetUserQuota(userId, false)
	require.NoError(t, err)
	assert.Equal(t, 1000, userQuota)
	assert.Equal(t, 1000, loadBillingRefundToken(t, tokenId).RemainQuota)
}

func TestBillingSessionRefundDoesNotRepeatTokenAfterFundingRetry(t *testing.T) {
	truncate(t)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousRedisEnabled := common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.RedisEnabled = previousRedisEnabled
	})

	const userId, tokenId, consumed = 8201, 8202, 100
	seedUser(t, userId, 1000)
	seedToken(t, tokenId, userId, "billing-session-funding-retry", 900)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenId).Update("used_quota", consumed).Error)
	funding := &billingRefundTestFunding{
		refundFn: func(call int32) error {
			if call == 1 {
				return errors.New("forced funding refund failure")
			}
			return nil
		},
	}
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId: userId, TokenId: tokenId, TokenKey: "billing-session-funding-retry",
		},
		funding:          funding,
		preConsumedQuota: consumed,
		tokenConsumed:    consumed,
	}

	session.Refund(billingRefundTestContext())
	requireBillingRefundState(t, session, billingRefundPending)
	assert.True(t, session.NeedsRefund())
	assert.EqualValues(t, 1, funding.calls.Load())
	session.mu.Lock()
	assert.False(t, session.fundingRefunded)
	assert.True(t, session.tokenRefunded)
	session.mu.Unlock()
	token := loadBillingRefundToken(t, tokenId)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)

	session.Refund(billingRefundTestContext())
	requireBillingRefundState(t, session, billingRefundCompleted)
	assert.EqualValues(t, 2, funding.calls.Load())
	token = loadBillingRefundToken(t, tokenId)
	assert.Equal(t, 1000, token.RemainQuota, "successful token refund must not run twice")
	assert.Zero(t, token.UsedQuota)
}

func TestBillingSessionConcurrentRefundStartsOneExecution(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	funding := &billingRefundTestFunding{
		refundFn: func(call int32) error {
			if call == 1 {
				close(started)
			}
			<-release
			return nil
		},
	}
	session := &BillingSession{
		relayInfo:     &relaycommon.RelayInfo{UserId: 8301, IsPlayground: true},
		funding:       funding,
		tokenConsumed: 100,
	}
	ctx := billingRefundTestContext()

	session.Refund(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "refund execution did not start")
	}

	var callers sync.WaitGroup
	for range 32 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			session.Refund(ctx)
		}()
	}
	callers.Wait()
	assert.EqualValues(t, 1, funding.calls.Load())
	session.mu.Lock()
	assert.Equal(t, billingRefundInFlight, session.refundState)
	session.mu.Unlock()
	assert.False(t, session.NeedsRefund())

	close(release)
	requireBillingRefundState(t, session, billingRefundCompleted)
	for range 16 {
		session.Refund(ctx)
	}
	assert.EqualValues(t, 1, funding.calls.Load())
}
