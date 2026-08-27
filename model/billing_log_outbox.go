package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BillingLogOutboxStatusPending   = "pending"
	BillingLogOutboxStatusLeased    = "leased"
	BillingLogOutboxStatusDelivered = "delivered"

	BillingLogOutboxPayloadVersion = 1
	billingLogOutboxMaxBatchSize   = 100
	billingLogOutboxMaxErrorLength = 4096
	billingLogOutboxMaxPayloadSize = 60 * 1024

	billingLogOutboxDispatchInterval = time.Second
	billingLogOutboxLeaseDuration    = 30 * time.Second
	billingLogOutboxRetryBaseDelay   = 2 * time.Second
	billingLogOutboxRetryMaxDelay    = 5 * time.Minute
)

var (
	ErrBillingLogOutboxInvalid             = errors.New("invalid billing log outbox event")
	ErrBillingLogOutboxPayloadTooLarge     = errors.New("billing log outbox payload is too large")
	ErrBillingLogOutboxFingerprintConflict = errors.New("billing log outbox payload fingerprint conflict")
	ErrBillingLogOutboxPayloadCorrupt      = errors.New("billing log outbox payload fingerprint mismatch")
	ErrBillingLogOutboxLeaseLost           = errors.New("billing log outbox lease lost")
	ErrBillingLogEventConflict             = errors.New("billing log event fingerprint conflict")
)

// BillingLogResourceSnapshot keeps mutable display data beside the stable ID
// so delayed delivery never has to re-read the current resource state.
type BillingLogResourceSnapshot struct {
	Id   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

type BillingLogReservationSnapshot struct {
	Id            int64  `json:"id"`
	Status        string `json:"status,omitempty"`
	ReservedQuota int    `json:"reserved_quota,omitempty"`
	SettledQuota  int    `json:"settled_quota,omitempty"`
}

type BillingLogLedgerSnapshot struct {
	Id        int64  `json:"id"`
	Operation string `json:"operation,omitempty"`
}

// BillingLogOutboxPayload is an immutable billing projection. Every value
// needed by the log sink is captured in the accounting transaction.
type BillingLogOutboxPayload struct {
	Version      int                           `json:"version"`
	LogType      int                           `json:"log_type"`
	Organization BillingLogResourceSnapshot    `json:"organization"`
	User         BillingLogResourceSnapshot    `json:"user"`
	Project      *BillingLogResourceSnapshot   `json:"project,omitempty"`
	Token        BillingLogResourceSnapshot    `json:"token"`
	Channel      BillingLogResourceSnapshot    `json:"channel"`
	Model        string                        `json:"model"`
	Group        string                        `json:"group"`
	Quota        int                           `json:"quota"`
	Reason       string                        `json:"reason"`
	Reservation  BillingLogReservationSnapshot `json:"reservation"`
	Ledger       BillingLogLedgerSnapshot      `json:"ledger"`
	RequestId    string                        `json:"request_id"`
	CreatedAt    int64                         `json:"created_at"`
	Other        map[string]interface{}        `json:"other,omitempty"`
}

// BillingLogOutbox lives in the primary database. It must be inserted with
// the balance mutation by EnqueueBillingLogOutboxTx.
type BillingLogOutbox struct {
	Id                 int64  `json:"id"`
	EventKey           string `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	Payload            string `json:"payload" gorm:"type:text;not null"`
	PayloadFingerprint string `json:"payload_fingerprint" gorm:"type:char(64);not null"`
	Status             string `json:"status" gorm:"type:varchar(16);not null;index:idx_billing_log_outbox_ready,priority:1"`
	Attempts           int    `json:"attempts" gorm:"not null"`
	LeaseOwner         string `json:"lease_owner" gorm:"type:varchar(64);not null"`
	LeaseUntil         int64  `json:"lease_until" gorm:"type:bigint;not null;index"`
	NextAttemptAt      int64  `json:"next_attempt_at" gorm:"type:bigint;not null;index:idx_billing_log_outbox_ready,priority:2"`
	LastError          string `json:"last_error" gorm:"type:text;not null"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null;index"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
	DeliveredAt        int64  `json:"delivered_at" gorm:"type:bigint;not null"`
}

type BillingLogOutboxDispatchResult struct {
	Claimed        int
	Delivered      int
	RetryScheduled int
}

type billingLogOutboxDispatcherConfig struct {
	BatchSize      int
	Interval       time.Duration
	LeaseDuration  time.Duration
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
}

var billingLogOutboxDispatcherOnce sync.Once

type billingLogEventMetadata struct {
	EventKey     string                        `json:"event_key"`
	Fingerprint  string                        `json:"fingerprint"`
	LogType      int                           `json:"log_type"`
	Organization BillingLogResourceSnapshot    `json:"organization"`
	User         BillingLogResourceSnapshot    `json:"user"`
	Project      *BillingLogResourceSnapshot   `json:"project,omitempty"`
	Token        BillingLogResourceSnapshot    `json:"token"`
	Channel      BillingLogResourceSnapshot    `json:"channel"`
	Model        string                        `json:"model"`
	Group        string                        `json:"group"`
	Quota        int                           `json:"quota"`
	Reservation  BillingLogReservationSnapshot `json:"reservation"`
	Ledger       BillingLogLedgerSnapshot      `json:"ledger"`
	Reason       string                        `json:"reason"`
	RequestId    string                        `json:"request_id"`
	CreatedAt    int64                         `json:"created_at"`
}

func billingLogPayloadFingerprint(payload string) string {
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func validBillingLogType(logType int) bool {
	return logType >= LogTypeTopup && logType <= LogTypeLogin
}

func validateBillingLogOutboxPayload(payload *BillingLogOutboxPayload) error {
	if payload == nil || payload.Version != BillingLogOutboxPayloadVersion || !validBillingLogType(payload.LogType) ||
		payload.Organization.Id <= 0 || payload.User.Id <= 0 || payload.Token.Id < 0 || payload.Channel.Id < 0 ||
		payload.Quota < 0 || payload.Quota > common.MaxQuota || payload.Reservation.Id < 0 ||
		payload.Reservation.ReservedQuota < 0 || payload.Reservation.ReservedQuota > common.MaxQuota ||
		payload.Reservation.SettledQuota < 0 || payload.Reservation.SettledQuota > common.MaxQuota ||
		payload.Ledger.Id < 0 || strings.TrimSpace(payload.RequestId) == "" || len(payload.RequestId) > 64 ||
		payload.CreatedAt <= 0 {
		return ErrBillingLogOutboxInvalid
	}
	if payload.Project != nil && (payload.Project.Id <= 0 || len(payload.Project.Name) > 128) {
		return ErrBillingLogOutboxInvalid
	}
	if len(payload.Organization.Name) > 128 || len(payload.User.Name) > 128 || len(payload.Token.Name) > 128 ||
		len(payload.Channel.Name) > 128 || len(payload.Model) > 255 || len(payload.Group) > 64 || len(payload.Reason) > 4096 ||
		len(payload.Reservation.Status) > 16 || len(payload.Ledger.Operation) > 32 {
		return ErrBillingLogOutboxInvalid
	}
	return nil
}

// EnqueueBillingLogOutboxTx atomically records an immutable log projection in
// the caller's primary-database transaction. Reusing an event key with the
// same payload is idempotent; changing the payload is a hard conflict.
func EnqueueBillingLogOutboxTx(tx *gorm.DB, eventKey string, payload BillingLogOutboxPayload) (*BillingLogOutbox, error) {
	eventKey = strings.TrimSpace(eventKey)
	if tx == nil || eventKey == "" || len(eventKey) > 128 {
		return nil, ErrBillingLogOutboxInvalid
	}
	if err := validateBillingLogOutboxPayload(&payload); err != nil {
		return nil, err
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal billing log outbox payload: %w", err)
	}
	if len(payloadBytes) > billingLogOutboxMaxPayloadSize {
		return nil, ErrBillingLogOutboxPayloadTooLarge
	}
	payloadText := string(payloadBytes)
	fingerprint := billingLogPayloadFingerprint(payloadText)
	now := common.GetTimestamp()
	candidate := &BillingLogOutbox{
		EventKey:           eventKey,
		Payload:            payloadText,
		PayloadFingerprint: fingerprint,
		Status:             BillingLogOutboxStatusPending,
		NextAttemptAt:      now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_key"}},
		DoNothing: true,
	}).Create(candidate).Error; err != nil {
		return nil, err
	}

	var stored BillingLogOutbox
	if err := lockForUpdate(tx).Where("event_key = ?", eventKey).First(&stored).Error; err != nil {
		return nil, err
	}
	if stored.PayloadFingerprint != fingerprint || stored.Payload != payloadText {
		return nil, ErrBillingLogOutboxFingerprintConflict
	}
	return &stored, nil
}

func billingLogOutboxReadyQuery(tx *gorm.DB, now int64) *gorm.DB {
	return tx.Where(
		"(status = ? AND next_attempt_at <= ?) OR (status = ? AND lease_until <= ?)",
		BillingLogOutboxStatusPending,
		now,
		BillingLogOutboxStatusLeased,
		now,
	)
}

func claimBillingLogOutboxAt(ctx context.Context, leaseOwner string, limit int, now int64, leaseSeconds int64) ([]BillingLogOutbox, error) {
	leaseOwner = strings.TrimSpace(leaseOwner)
	if DB == nil || leaseOwner == "" || len(leaseOwner) > 64 || limit <= 0 || leaseSeconds <= 0 {
		return nil, ErrBillingLogOutboxInvalid
	}
	if limit > billingLogOutboxMaxBatchSize {
		limit = billingLogOutboxMaxBatchSize
	}

	var candidates []BillingLogOutbox
	if err := billingLogOutboxReadyQuery(DB.WithContext(ctx), now).
		Order("id asc").
		Limit(limit).
		Find(&candidates).Error; err != nil {
		return nil, err
	}

	claimed := make([]BillingLogOutbox, 0, len(candidates))
	leaseUntil := now + leaseSeconds
	for i := range candidates {
		candidate := candidates[i]
		result := billingLogOutboxReadyQuery(DB.WithContext(ctx).Model(&BillingLogOutbox{}).Where("id = ?", candidate.Id), now).
			Updates(map[string]interface{}{
				"status":      BillingLogOutboxStatusLeased,
				"attempts":    gorm.Expr("attempts + ?", 1),
				"lease_owner": leaseOwner,
				"lease_until": leaseUntil,
				"updated_at":  now,
			})
		if result.Error != nil {
			return claimed, result.Error
		}
		if result.RowsAffected != 1 {
			continue
		}
		candidate.Status = BillingLogOutboxStatusLeased
		candidate.Attempts++
		candidate.LeaseOwner = leaseOwner
		candidate.LeaseUntil = leaseUntil
		candidate.UpdatedAt = now
		claimed = append(claimed, candidate)
	}
	return claimed, nil
}

// ClaimBillingLogOutbox leases ready events with a compare-and-swap update.
// Expired leases are ready immediately, including events whose retry time was
// set later than the abandoned lease.
func ClaimBillingLogOutbox(ctx context.Context, leaseOwner string, limit int, leaseDuration time.Duration) ([]BillingLogOutbox, error) {
	if leaseDuration <= 0 {
		return nil, ErrBillingLogOutboxInvalid
	}
	leaseSeconds := int64((leaseDuration + time.Second - 1) / time.Second)
	return claimBillingLogOutboxAt(ctx, leaseOwner, limit, common.GetTimestamp(), leaseSeconds)
}

func decodeBillingLogOutboxPayload(event *BillingLogOutbox) (*BillingLogOutboxPayload, error) {
	if event == nil || strings.TrimSpace(event.EventKey) == "" || len(event.EventKey) > 128 ||
		event.Payload == "" || event.PayloadFingerprint == "" {
		return nil, ErrBillingLogOutboxInvalid
	}
	if billingLogPayloadFingerprint(event.Payload) != event.PayloadFingerprint {
		return nil, ErrBillingLogOutboxPayloadCorrupt
	}
	var payload BillingLogOutboxPayload
	if err := common.UnmarshalJsonStr(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode billing log outbox payload: %w", err)
	}
	if err := validateBillingLogOutboxPayload(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func billingLogEventExists(ctx context.Context, eventKey string, fingerprint string) (bool, error) {
	var matches []Log
	err := LOG_DB.WithContext(ctx).
		Select("billing_event_id", "billing_event_fingerprint").
		Where("billing_event_id = ?", eventKey).
		Limit(10).
		Find(&matches).Error
	if err != nil {
		return false, err
	}
	if len(matches) == 0 {
		return false, nil
	}
	for i := range matches {
		if matches[i].BillingEventFingerprint == nil || *matches[i].BillingEventFingerprint != fingerprint {
			return false, ErrBillingLogEventConflict
		}
	}
	return true, nil
}

// DeliverBillingLogOutbox writes one leased event to the configured log sink.
// SQL sinks enforce billing_event_id uniqueness. ClickHouse cannot enforce a
// unique key on MergeTree, so delivery checks the deterministic ID first and
// readers/exporters can use the same non-empty ID as their deduplication key.
func DeliverBillingLogOutbox(ctx context.Context, event *BillingLogOutbox) error {
	if LOG_DB == nil {
		return errors.New("log database is not initialized")
	}
	payload, err := decodeBillingLogOutboxPayload(event)
	if err != nil {
		return err
	}

	exists, err := billingLogEventExists(ctx, event.EventKey, event.PayloadFingerprint)
	if err != nil || exists {
		return err
	}
	metadata := billingLogEventMetadata{
		EventKey:     event.EventKey,
		Fingerprint:  event.PayloadFingerprint,
		LogType:      payload.LogType,
		Organization: payload.Organization,
		User:         payload.User,
		Project:      payload.Project,
		Token:        payload.Token,
		Channel:      payload.Channel,
		Model:        payload.Model,
		Group:        payload.Group,
		Quota:        payload.Quota,
		Reservation:  payload.Reservation,
		Ledger:       payload.Ledger,
		Reason:       payload.Reason,
		RequestId:    payload.RequestId,
		CreatedAt:    payload.CreatedAt,
	}
	other := make(map[string]interface{}, len(payload.Other)+1)
	for key, value := range payload.Other {
		other[key] = value
	}
	other["billing_event"] = metadata
	otherBytes, err := common.Marshal(other)
	if err != nil {
		return fmt.Errorf("marshal billing log metadata: %w", err)
	}
	eventID := event.EventKey
	fingerprint := event.PayloadFingerprint
	log := &Log{
		UserId:                  payload.User.Id,
		OrganizationId:          payload.Organization.Id,
		ProjectId:               nil,
		CreatedAt:               payload.CreatedAt,
		Type:                    payload.LogType,
		Content:                 payload.Reason,
		Username:                payload.User.Name,
		TokenName:               payload.Token.Name,
		ModelName:               payload.Model,
		Quota:                   payload.Quota,
		ChannelId:               payload.Channel.Id,
		ChannelName:             payload.Channel.Name,
		TokenId:                 payload.Token.Id,
		Group:                   payload.Group,
		RequestId:               payload.RequestId,
		Other:                   string(otherBytes),
		BillingEventId:          &eventID,
		BillingEventFingerprint: &fingerprint,
	}
	if payload.Project != nil {
		projectID := payload.Project.Id
		log.ProjectId = &projectID
	}

	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return LOG_DB.WithContext(ctx).Create(log).Error
	}
	result := LOG_DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "billing_event_id"}},
		DoNothing: true,
	}).Create(log)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	exists, err = billingLogEventExists(ctx, event.EventKey, event.PayloadFingerprint)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("billing log sink did not persist event")
	}
	return nil
}

func markBillingLogOutboxDeliveredAt(ctx context.Context, event *BillingLogOutbox, now int64) error {
	if DB == nil || event == nil || event.Id <= 0 || event.Status != BillingLogOutboxStatusLeased ||
		event.LeaseOwner == "" || event.LeaseUntil <= 0 || event.Attempts <= 0 {
		return ErrBillingLogOutboxInvalid
	}
	result := DB.WithContext(ctx).Model(&BillingLogOutbox{}).
		Where("id = ? AND status = ? AND lease_owner = ? AND lease_until = ? AND attempts = ?",
			event.Id, BillingLogOutboxStatusLeased, event.LeaseOwner, event.LeaseUntil, event.Attempts).
		Updates(map[string]interface{}{
			"status":       BillingLogOutboxStatusDelivered,
			"lease_owner":  "",
			"lease_until":  0,
			"last_error":   "",
			"delivered_at": now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBillingLogOutboxLeaseLost
	}
	return nil
}

func MarkBillingLogOutboxDelivered(ctx context.Context, event *BillingLogOutbox) error {
	return markBillingLogOutboxDeliveredAt(ctx, event, common.GetTimestamp())
}

func markBillingLogOutboxRetryAt(ctx context.Context, event *BillingLogOutbox, now int64, nextAttemptAt int64, deliveryErr error) error {
	if DB == nil || event == nil || event.Id <= 0 || event.Status != BillingLogOutboxStatusLeased ||
		event.LeaseOwner == "" || event.LeaseUntil <= 0 || event.Attempts <= 0 || nextAttemptAt < now {
		return ErrBillingLogOutboxInvalid
	}
	lastError := "delivery failed"
	if deliveryErr != nil {
		lastError = deliveryErr.Error()
	}
	if len(lastError) > billingLogOutboxMaxErrorLength {
		runes := []rune(lastError)
		if len(runes) > billingLogOutboxMaxErrorLength {
			lastError = string(runes[:billingLogOutboxMaxErrorLength])
		}
	}
	result := DB.WithContext(ctx).Model(&BillingLogOutbox{}).
		Where("id = ? AND status = ? AND lease_owner = ? AND lease_until = ? AND attempts = ?",
			event.Id, BillingLogOutboxStatusLeased, event.LeaseOwner, event.LeaseUntil, event.Attempts).
		Updates(map[string]interface{}{
			"status":          BillingLogOutboxStatusPending,
			"lease_owner":     "",
			"lease_until":     0,
			"next_attempt_at": nextAttemptAt,
			"last_error":      lastError,
			"updated_at":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBillingLogOutboxLeaseLost
	}
	return nil
}

func MarkBillingLogOutboxRetry(ctx context.Context, event *BillingLogOutbox, retryDelay time.Duration, deliveryErr error) error {
	if retryDelay < 0 {
		return ErrBillingLogOutboxInvalid
	}
	now := common.GetTimestamp()
	retrySeconds := int64((retryDelay + time.Second - 1) / time.Second)
	return markBillingLogOutboxRetryAt(ctx, event, now, now+retrySeconds, deliveryErr)
}

// DispatchBillingLogOutbox performs one bounded claim/deliver/ack pass. A log
// write followed by an ack failure is intentionally left leased: after lease
// expiry, deterministic sink deduplication makes the retry safe.
func DispatchBillingLogOutbox(ctx context.Context, leaseOwner string, limit int, leaseDuration time.Duration, retryDelay time.Duration) (BillingLogOutboxDispatchResult, error) {
	return dispatchBillingLogOutboxWithBackoff(ctx, leaseOwner, limit, leaseDuration, retryDelay, retryDelay)
}

func billingLogOutboxRetryDelay(attempt int, baseDelay time.Duration, maxDelay time.Duration) time.Duration {
	if baseDelay <= 0 {
		return 0
	}
	if maxDelay < baseDelay {
		maxDelay = baseDelay
	}
	delay := baseDelay
	for retry := 1; retry < attempt; retry++ {
		if delay >= maxDelay || delay > maxDelay/2 {
			return maxDelay
		}
		delay *= 2
	}
	return delay
}

func dispatchBillingLogOutboxWithBackoff(ctx context.Context, leaseOwner string, limit int, leaseDuration time.Duration, retryBaseDelay time.Duration, retryMaxDelay time.Duration) (BillingLogOutboxDispatchResult, error) {
	var dispatch BillingLogOutboxDispatchResult
	if retryBaseDelay < 0 || retryMaxDelay < retryBaseDelay {
		return dispatch, ErrBillingLogOutboxInvalid
	}
	events, err := ClaimBillingLogOutbox(ctx, leaseOwner, limit, leaseDuration)
	if err != nil {
		return dispatch, err
	}
	dispatch.Claimed = len(events)
	var deliveryErrors []error
	for i := range events {
		event := &events[i]
		if err := DeliverBillingLogOutbox(ctx, event); err != nil {
			retryDelay := billingLogOutboxRetryDelay(event.Attempts, retryBaseDelay, retryMaxDelay)
			if retryErr := MarkBillingLogOutboxRetry(ctx, event, retryDelay, err); retryErr != nil {
				deliveryErrors = append(deliveryErrors, fmt.Errorf("deliver %s: %w; schedule retry: %v", event.EventKey, err, retryErr))
				continue
			}
			dispatch.RetryScheduled++
			deliveryErrors = append(deliveryErrors, fmt.Errorf("deliver %s: %w", event.EventKey, err))
			continue
		}
		if err := MarkBillingLogOutboxDelivered(ctx, event); err != nil {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("acknowledge %s: %w", event.EventKey, err))
			continue
		}
		dispatch.Delivered++
	}
	return dispatch, errors.Join(deliveryErrors...)
}

func runBillingLogOutboxDispatcher(ctx context.Context, leaseOwner string, config billingLogOutboxDispatcherConfig) {
	if config.BatchSize <= 0 || config.BatchSize > billingLogOutboxMaxBatchSize || config.Interval <= 0 ||
		config.LeaseDuration <= 0 || config.RetryBaseDelay < 0 || config.RetryMaxDelay < config.RetryBaseDelay {
		common.SysError("billing log outbox dispatcher has invalid configuration")
		return
	}
	runPass := func() {
		for {
			result, err := dispatchBillingLogOutboxWithBackoff(
				ctx,
				leaseOwner,
				config.BatchSize,
				config.LeaseDuration,
				config.RetryBaseDelay,
				config.RetryMaxDelay,
			)
			if err != nil {
				common.SysError("billing log outbox dispatch failed: " + err.Error())
			}
			if result.Claimed < config.BatchSize {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	runPass()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runPass()
		}
	}
}

func startBillingLogOutboxDispatcher(ctx context.Context, config billingLogOutboxDispatcherConfig) bool {
	if !common.IsMasterNode {
		return false
	}
	leaseOwner := "billing-log-" + common.NewRequestId()
	go runBillingLogOutboxDispatcher(ctx, leaseOwner, config)
	return true
}

// StartBillingLogOutboxDispatcher starts the master-only at-least-once log
// projection worker. Call it once after both DB and LOG_DB are initialized.
func StartBillingLogOutboxDispatcher() {
	if !common.IsMasterNode {
		return
	}
	billingLogOutboxDispatcherOnce.Do(func() {
		startBillingLogOutboxDispatcher(context.Background(), billingLogOutboxDispatcherConfig{
			BatchSize:      billingLogOutboxMaxBatchSize,
			Interval:       billingLogOutboxDispatchInterval,
			LeaseDuration:  billingLogOutboxLeaseDuration,
			RetryBaseDelay: billingLogOutboxRetryBaseDelay,
			RetryMaxDelay:  billingLogOutboxRetryMaxDelay,
		})
	})
}
