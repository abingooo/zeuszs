package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type billingLogOutboxTestMutation struct {
	Id    int64
	Value string
}

func openBillingLogOutboxSQLite(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL", filepath.Join(t.TempDir(), name))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(10)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func setupBillingLogOutboxTestDatabases(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	mainDB := openBillingLogOutboxSQLite(t, "main.db")
	logDB := openBillingLogOutboxSQLite(t, "log.db")
	require.NoError(t, mainDB.AutoMigrate(&BillingLogOutbox{}))
	require.NoError(t, logDB.AutoMigrate(&Log{}))

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	return mainDB, logDB
}

func billingLogOutboxTestPayload(seed int) BillingLogOutboxPayload {
	projectID := 300 + seed
	return BillingLogOutboxPayload{
		Version: BillingLogOutboxPayloadVersion,
		LogType: LogTypeConsume,
		Organization: BillingLogResourceSnapshot{
			Id:   10 + seed,
			Name: fmt.Sprintf("organization-%d", seed),
		},
		User: BillingLogResourceSnapshot{
			Id:   20 + seed,
			Name: fmt.Sprintf("user-%d", seed),
		},
		Project: &BillingLogResourceSnapshot{
			Id:   projectID,
			Name: fmt.Sprintf("project-%d", seed),
		},
		Token: BillingLogResourceSnapshot{
			Id:   40 + seed,
			Name: fmt.Sprintf("token-%d", seed),
		},
		Channel: BillingLogResourceSnapshot{
			Id:   50 + seed,
			Name: fmt.Sprintf("channel-%d", seed),
		},
		Model:  fmt.Sprintf("model-%d", seed),
		Group:  fmt.Sprintf("group-%d", seed),
		Quota:  60 + seed,
		Reason: fmt.Sprintf("settlement-%d", seed),
		Reservation: BillingLogReservationSnapshot{
			Id:            int64(70 + seed),
			Status:        OrganizationWalletReservationSettled,
			ReservedQuota: 100 + seed,
			SettledQuota:  60 + seed,
		},
		Ledger: BillingLogLedgerSnapshot{
			Id:        int64(80 + seed),
			Operation: OrganizationLedgerSettle,
		},
		RequestId: fmt.Sprintf("billing-request-%d", seed),
		CreatedAt: 1_700_000_000 + int64(seed),
		Other: map[string]interface{}{
			"task_id": fmt.Sprintf("task-%d", seed),
		},
	}
}

func enqueueBillingLogOutboxTestEvent(t *testing.T, eventKey string, payload BillingLogOutboxPayload) *BillingLogOutbox {
	t.Helper()
	var event *BillingLogOutbox
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		event, err = EnqueueBillingLogOutboxTx(tx, eventKey, payload)
		return err
	}))
	return event
}

func TestBillingLogOutboxAtomicInsertAndFingerprintConflict(t *testing.T) {
	mainDB, _ := setupBillingLogOutboxTestDatabases(t)
	require.NoError(t, mainDB.AutoMigrate(&billingLogOutboxTestMutation{}))
	payload := billingLogOutboxTestPayload(1)
	rollbackErr := errors.New("rollback accounting mutation")

	err := mainDB.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&billingLogOutboxTestMutation{Value: "rolled-back"}).Error)
		_, enqueueErr := EnqueueBillingLogOutboxTx(tx, "billing:event:atomic", payload)
		require.NoError(t, enqueueErr)
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	var mutationCount, outboxCount int64
	require.NoError(t, mainDB.Model(&billingLogOutboxTestMutation{}).Count(&mutationCount).Error)
	require.NoError(t, mainDB.Model(&BillingLogOutbox{}).Count(&outboxCount).Error)
	assert.Zero(t, mutationCount)
	assert.Zero(t, outboxCount)

	require.NoError(t, mainDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&billingLogOutboxTestMutation{Value: "committed"}).Error; err != nil {
			return err
		}
		_, err := EnqueueBillingLogOutboxTx(tx, "billing:event:atomic", payload)
		return err
	}))
	enqueueBillingLogOutboxTestEvent(t, "billing:event:atomic", payload)
	require.NoError(t, mainDB.Model(&BillingLogOutbox{}).Count(&outboxCount).Error)
	assert.EqualValues(t, 1, outboxCount)

	changed := payload
	changed.Quota++
	err = mainDB.Transaction(func(tx *gorm.DB) error {
		_, enqueueErr := EnqueueBillingLogOutboxTx(tx, "billing:event:atomic", changed)
		return enqueueErr
	})
	require.ErrorIs(t, err, ErrBillingLogOutboxFingerprintConflict)

	var stored BillingLogOutbox
	require.NoError(t, mainDB.Where("event_key = ?", "billing:event:atomic").First(&stored).Error)
	decoded, err := decodeBillingLogOutboxPayload(&stored)
	require.NoError(t, err)
	assert.Equal(t, payload.Quota, decoded.Quota)
}

func TestBillingLogOutboxRejectsPayloadBeyondPortableTextLimit(t *testing.T) {
	mainDB, _ := setupBillingLogOutboxTestDatabases(t)
	payload := billingLogOutboxTestPayload(9)
	payload.Other["oversized"] = strings.Repeat("x", billingLogOutboxMaxPayloadSize)

	err := mainDB.Transaction(func(tx *gorm.DB) error {
		_, enqueueErr := EnqueueBillingLogOutboxTx(tx, "billing:event:oversized", payload)
		return enqueueErr
	})
	require.ErrorIs(t, err, ErrBillingLogOutboxPayloadTooLarge)

	var count int64
	require.NoError(t, mainDB.Model(&BillingLogOutbox{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingLogOutboxLogFailureReturnsToPendingAndRetries(t *testing.T) {
	mainDB, logDB := setupBillingLogOutboxTestDatabases(t)
	enqueueBillingLogOutboxTestEvent(t, "billing:event:retry", billingLogOutboxTestPayload(2))
	now := common.GetTimestamp() + 5
	claimed, err := claimBillingLogOutboxAt(context.Background(), "worker-a", 1, now, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	require.NoError(t, logDB.Migrator().DropTable(&Log{}))
	deliveryErr := DeliverBillingLogOutbox(context.Background(), &claimed[0])
	require.Error(t, deliveryErr)
	require.NoError(t, markBillingLogOutboxRetryAt(context.Background(), &claimed[0], now+1, now+11, deliveryErr))

	var pending BillingLogOutbox
	require.NoError(t, mainDB.First(&pending, claimed[0].Id).Error)
	assert.Equal(t, BillingLogOutboxStatusPending, pending.Status)
	assert.Equal(t, 1, pending.Attempts)
	assert.Equal(t, now+11, pending.NextAttemptAt)
	assert.NotEmpty(t, pending.LastError)

	notReady, err := claimBillingLogOutboxAt(context.Background(), "worker-b", 1, now+10, 10)
	require.NoError(t, err)
	assert.Empty(t, notReady)
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	retry, err := claimBillingLogOutboxAt(context.Background(), "worker-b", 1, now+11, 10)
	require.NoError(t, err)
	require.Len(t, retry, 1)
	assert.Equal(t, 2, retry[0].Attempts)
	require.NoError(t, DeliverBillingLogOutbox(context.Background(), &retry[0]))
	require.NoError(t, markBillingLogOutboxDeliveredAt(context.Background(), &retry[0], now+12))

	var delivered BillingLogOutbox
	require.NoError(t, mainDB.First(&delivered, retry[0].Id).Error)
	assert.Equal(t, BillingLogOutboxStatusDelivered, delivered.Status)
	assert.Equal(t, now+12, delivered.DeliveredAt)
}

func TestBillingLogOutboxCrashBeforeAckRedeliveryDoesNotDuplicateSQLLog(t *testing.T) {
	mainDB, logDB := setupBillingLogOutboxTestDatabases(t)
	payload := billingLogOutboxTestPayload(3)
	enqueueBillingLogOutboxTestEvent(t, "billing:event:crash", payload)
	now := common.GetTimestamp() + 5
	first, err := claimBillingLogOutboxAt(context.Background(), "worker-a", 1, now, 5)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.NoError(t, DeliverBillingLogOutbox(context.Background(), &first[0]))

	second, err := claimBillingLogOutboxAt(context.Background(), "worker-b", 1, now+5, 5)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, 2, second[0].Attempts)
	require.NoError(t, DeliverBillingLogOutbox(context.Background(), &second[0]))
	require.NoError(t, markBillingLogOutboxDeliveredAt(context.Background(), &second[0], now+6))

	var logCount int64
	require.NoError(t, logDB.Model(&Log{}).Where("billing_event_id = ?", "billing:event:crash").Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)
	var deliveredLog Log
	require.NoError(t, logDB.Where("billing_event_id = ?", "billing:event:crash").First(&deliveredLog).Error)
	require.NotNil(t, deliveredLog.BillingEventFingerprint)
	assert.Equal(t, first[0].PayloadFingerprint, *deliveredLog.BillingEventFingerprint)
	assert.Equal(t, payload.Organization.Id, deliveredLog.OrganizationId)
	assert.Equal(t, payload.User.Id, deliveredLog.UserId)
	require.NotNil(t, deliveredLog.ProjectId)
	assert.Equal(t, payload.Project.Id, *deliveredLog.ProjectId)
	assert.Equal(t, payload.Token.Id, deliveredLog.TokenId)
	assert.Equal(t, payload.Channel.Id, deliveredLog.ChannelId)
	assert.Equal(t, payload.Model, deliveredLog.ModelName)
	assert.Equal(t, payload.Group, deliveredLog.Group)
	assert.Equal(t, payload.Quota, deliveredLog.Quota)
	assert.Equal(t, payload.Reason, deliveredLog.Content)

	var delivered BillingLogOutbox
	require.NoError(t, mainDB.First(&delivered, second[0].Id).Error)
	assert.Equal(t, BillingLogOutboxStatusDelivered, delivered.Status)
}

func TestBillingLogOutboxConcurrentLeaseAndExpiry(t *testing.T) {
	mainDB, _ := setupBillingLogOutboxTestDatabases(t)
	const eventCount = 12
	for i := 0; i < eventCount; i++ {
		enqueueBillingLogOutboxTestEvent(t, fmt.Sprintf("billing:event:lease:%d", i), billingLogOutboxTestPayload(100+i))
	}
	now := common.GetTimestamp() + 5
	start := make(chan struct{})
	results := make(chan []BillingLogOutbox, 2)
	errorsCh := make(chan error, 2)
	var workers sync.WaitGroup
	for _, owner := range []string{"worker-a", "worker-b"} {
		owner := owner
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			claimed, err := claimBillingLogOutboxAt(context.Background(), owner, eventCount, now, 5)
			results <- claimed
			errorsCh <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}

	claimedByID := make(map[int64]BillingLogOutbox, eventCount)
	for batch := range results {
		for _, event := range batch {
			_, duplicate := claimedByID[event.Id]
			assert.False(t, duplicate, "event %d was leased by two workers", event.Id)
			claimedByID[event.Id] = event
		}
	}
	assert.Len(t, claimedByID, eventCount)

	var leasedRows []BillingLogOutbox
	require.NoError(t, mainDB.Order("id asc").Find(&leasedRows).Error)
	require.Len(t, leasedRows, eventCount)
	for i := range leasedRows {
		assert.Equal(t, BillingLogOutboxStatusLeased, leasedRows[i].Status)
		assert.Equal(t, 1, leasedRows[i].Attempts)
	}

	staleClaim := claimedByID[leasedRows[0].Id]
	reclaimed, err := claimBillingLogOutboxAt(context.Background(), "worker-c", 1, now+5, 5)
	require.NoError(t, err)
	require.Len(t, reclaimed, 1)
	assert.Equal(t, staleClaim.Id, reclaimed[0].Id)
	assert.Equal(t, 2, reclaimed[0].Attempts)
	assert.Equal(t, "worker-c", reclaimed[0].LeaseOwner)
	require.ErrorIs(t, markBillingLogOutboxDeliveredAt(context.Background(), &staleClaim, now+6), ErrBillingLogOutboxLeaseLost)
	require.NoError(t, markBillingLogOutboxDeliveredAt(context.Background(), &reclaimed[0], now+6))
}

func TestBillingLogOutboxRetryDelayIsExponentialAndCapped(t *testing.T) {
	assert.Zero(t, billingLogOutboxRetryDelay(5, 0, time.Minute))
	assert.Equal(t, 2*time.Second, billingLogOutboxRetryDelay(1, 2*time.Second, time.Minute))
	assert.Equal(t, 4*time.Second, billingLogOutboxRetryDelay(2, 2*time.Second, time.Minute))
	assert.Equal(t, 8*time.Second, billingLogOutboxRetryDelay(3, 2*time.Second, time.Minute))
	assert.Equal(t, time.Minute, billingLogOutboxRetryDelay(20, 2*time.Second, time.Minute))
}

func TestBillingLogOutboxDispatcherAutomaticallyRetries(t *testing.T) {
	mainDB, logDB := setupBillingLogOutboxTestDatabases(t)
	enqueueBillingLogOutboxTestEvent(t, "billing:event:dispatcher", billingLogOutboxTestPayload(500))
	require.NoError(t, logDB.Migrator().DropTable(&Log{}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runBillingLogOutboxDispatcher(ctx, "dispatcher-test", billingLogOutboxDispatcherConfig{
			BatchSize:      10,
			Interval:       10 * time.Millisecond,
			LeaseDuration:  time.Second,
			RetryBaseDelay: 0,
			RetryMaxDelay:  0,
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	require.Eventually(t, func() bool {
		var event BillingLogOutbox
		if err := mainDB.Where("event_key = ?", "billing:event:dispatcher").First(&event).Error; err != nil {
			return false
		}
		return event.Status == BillingLogOutboxStatusPending && event.Attempts >= 1 && event.LastError != ""
	}, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, logDB.AutoMigrate(&Log{}))

	require.Eventually(t, func() bool {
		var event BillingLogOutbox
		if err := mainDB.Where("event_key = ?", "billing:event:dispatcher").First(&event).Error; err != nil {
			return false
		}
		return event.Status == BillingLogOutboxStatusDelivered && event.Attempts >= 2
	}, 2*time.Second, 10*time.Millisecond)
	var count int64
	require.NoError(t, logDB.Model(&Log{}).Where("billing_event_id = ?", "billing:event:dispatcher").Count(&count).Error)
	assert.EqualValues(t, 1, count)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("billing log outbox dispatcher did not stop after cancellation")
	}
}
