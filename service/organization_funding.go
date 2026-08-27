package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// OrganizationWalletFunding is the wallet implementation for a tenant-aware
// billing session. It keeps the FundingSource surface used by the relay while
// delegating every balance mutation to the durable organization reservation
// state machine in model.
//
// The actor is deliberately supplied by the trusted billing service, not by a
// request field. A constructor should be called only after authentication and
// organization membership have been resolved server-side.
type OrganizationWalletFunding struct {
	organizationId int
	userId         int
	requestId      string
	actor          model.OrganizationAccountingActor

	mu          sync.Mutex
	reservation *model.OrganizationWalletReservation
	consumed    int64
	settled     bool
	refunded    bool
}

var ErrOrganizationWalletFundingNotReserved = errors.New("organization wallet funding has not been reserved")

func NewOrganizationWalletFunding(organizationId int, userId int, requestId string, actor model.OrganizationAccountingActor) (*OrganizationWalletFunding, error) {
	if organizationId <= 0 || userId <= 0 || strings.TrimSpace(requestId) == "" || len(requestId) > 64 {
		return nil, model.ErrOrganizationAccountingInvalid
	}
	if actor.Kind == "" {
		return nil, model.ErrOrganizationAccountingInvalid
	}
	return &OrganizationWalletFunding{
		organizationId: organizationId,
		userId:         userId,
		requestId:      requestId,
		actor:          actor,
	}, nil
}

func (w *OrganizationWalletFunding) Source() string { return BillingSourceWallet }

func (w *OrganizationWalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.refunded || w.settled {
		return model.ErrOrganizationReservationState
	}
	if w.reservation != nil {
		if w.consumed == int64(amount) {
			return nil
		}
		return model.ErrOrganizationAccountingIdempotency
	}
	reservationResult, err := model.ReserveOrganizationWalletQuota(model.OrganizationWalletReserveParams{
		OrganizationId: w.organizationId,
		UserId:         w.userId,
		Amount:         int64(amount),
		RequestId:      w.requestId,
		IdempotencyKey: organizationFundingIdempotencyKey(w.requestId, "reserve"),
		SourceType:     "relay",
		SourceId:       w.requestId,
		Actor:          w.actor,
	})
	if err != nil {
		if errors.Is(err, model.ErrOrganizationUserQuotaInsufficient) {
			return ErrInsufficientWalletQuota
		}
		return err
	}
	w.reservation = &reservationResult.Reservation
	w.consumed = int64(amount)
	return nil
}

// ReserveTo extends the currently open reservation to targetQuota. It is
// deliberately separate from PreConsume: BillingSession may call Reserve
// several times while an auto-group retry discovers a more expensive route.
func (w *OrganizationWalletFunding) ReserveTo(targetQuota int) error {
	if targetQuota < 0 {
		return model.ErrOrganizationAccountingInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.refunded || w.settled {
		return model.ErrOrganizationReservationState
	}
	if int64(targetQuota) <= w.consumed {
		return nil
	}
	delta := int64(targetQuota) - w.consumed
	if delta > int64(common.MaxQuota-1)-w.consumed {
		return model.ErrOrganizationUserQuotaLimit
	}
	if w.reservation == nil {
		// A zero estimate has no durable reservation. Create one lazily when a
		// later routing decision requires a positive amount.
		reservationResult, err := model.ReserveOrganizationWalletQuota(model.OrganizationWalletReserveParams{
			OrganizationId: w.organizationId,
			UserId:         w.userId,
			Amount:         int64(targetQuota),
			RequestId:      w.requestId,
			IdempotencyKey: organizationFundingIdempotencyKey(w.requestId, fmt.Sprintf("reserve-to-%d", targetQuota)),
			SourceType:     "relay",
			SourceId:       w.requestId,
			Actor:          w.actor,
		})
		if err != nil {
			if errors.Is(err, model.ErrOrganizationUserQuotaInsufficient) {
				return ErrInsufficientWalletQuota
			}
			return err
		}
		w.reservation = &reservationResult.Reservation
		w.consumed = int64(targetQuota)
		return nil
	}

	reservationResult, err := model.ReserveAdditionalOrganizationWalletQuota(model.OrganizationWalletReservationIncreaseParams{
		ReservationId:  w.reservation.Id,
		Amount:         delta,
		IdempotencyKey: organizationFundingIdempotencyKey(w.requestId, fmt.Sprintf("reserve-to-%d", targetQuota)),
		RequestId:      w.requestId,
		Actor:          w.actor,
	})
	if err != nil {
		if errors.Is(err, model.ErrOrganizationUserQuotaInsufficient) {
			return ErrInsufficientWalletQuota
		}
		return err
	}
	w.reservation = &reservationResult.Reservation
	w.consumed = int64(targetQuota)
	return nil
}

func (w *OrganizationWalletFunding) Settle(delta int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.refunded {
		return model.ErrOrganizationReservationState
	}
	actualQuota := w.consumed + int64(delta)
	if actualQuota < 0 || actualQuota > int64(common.MaxQuota-1) {
		return model.ErrOrganizationAccountingInvalid
	}
	if w.reservation == nil {
		if actualQuota == 0 {
			// A zero estimate and zero actual usage is a valid no-op. Mark the
			// in-memory session settled without creating a ledger row.
			w.settled = true
			return nil
		}
		// A caller may start with a zero estimate (for example, a free group)
		// and receive a positive actual charge. Establish the reservation at
		// settlement time so the charge still uses the durable accounting
		// state machine instead of a legacy direct balance mutation.
		reservationResult, err := model.ReserveOrganizationWalletQuota(model.OrganizationWalletReserveParams{
			OrganizationId: w.organizationId,
			UserId:         w.userId,
			Amount:         actualQuota,
			RequestId:      w.requestId,
			IdempotencyKey: organizationFundingIdempotencyKey(w.requestId, fmt.Sprintf("reserve-to-%d", actualQuota)),
			SourceType:     "relay",
			SourceId:       w.requestId,
			Actor:          w.actor,
		})
		if err != nil {
			if errors.Is(err, model.ErrOrganizationUserQuotaInsufficient) {
				return ErrInsufficientWalletQuota
			}
			return err
		}
		w.reservation = &reservationResult.Reservation
		w.consumed = actualQuota
		delta = 0
	}
	if w.settled {
		if actualQuota == w.reservation.SettledQuota {
			return nil
		}
		return model.ErrOrganizationAccountingIdempotency
	}
	settled, err := model.SettleOrganizationWalletQuota(model.OrganizationWalletSettleParams{
		ReservationId:  w.reservation.Id,
		ActualQuota:    actualQuota,
		IdempotencyKey: organizationFundingIdempotencyKey(w.requestId, "settle"),
		RequestId:      w.requestId,
		Actor:          w.actor,
	})
	if err != nil {
		if errors.Is(err, model.ErrOrganizationUserQuotaInsufficient) {
			return ErrInsufficientWalletQuota
		}
		return err
	}
	w.reservation = &settled.Reservation
	w.consumed = actualQuota
	w.settled = true
	return nil
}

func (w *OrganizationWalletFunding) Refund() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.reservation == nil {
		return nil
	}
	if w.refunded {
		return nil
	}
	refunded, err := model.RefundOrganizationWalletQuota(model.OrganizationWalletRefundParams{
		ReservationId:  w.reservation.Id,
		IdempotencyKey: organizationFundingIdempotencyKey(w.requestId, "refund"),
		RequestId:      w.requestId,
		Actor:          w.actor,
	})
	if err != nil {
		return err
	}
	w.reservation = &refunded.Reservation
	w.refunded = true
	return nil
}

func (w *OrganizationWalletFunding) Reservation() (model.OrganizationWalletReservation, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.reservation == nil {
		return model.OrganizationWalletReservation{}, false
	}
	return *w.reservation, true
}

func organizationFundingIdempotencyKey(requestId string, phase string) string {
	key := fmt.Sprintf("wallet:%s:%s", requestId, phase)
	if len(key) <= 128 {
		return key
	}
	// Request IDs are normally short UUIDs. Hash an unusually long composite
	// key instead of truncating it, preserving collision resistance while
	// satisfying the ledger column limit.
	digest := sha256.Sum256([]byte(key))
	return "wallet:" + hex.EncodeToString(digest[:])
}
