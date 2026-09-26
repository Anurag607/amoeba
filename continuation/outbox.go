package continuation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxLeased    OutboxStatus = "leased"
	OutboxDelivered OutboxStatus = "delivered"
)

type OutboxLease struct {
	ID        string    `json:"id"`
	WorkerID  string    `json:"worker_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

var (
	ErrOutboxLeaseConflict = errors.New("continuation outbox lease conflict")
	ErrOutboxAttempt       = errors.New("continuation outbox attempt conflict")
)

// DeliveryPolicy controls at-least-once outbox delivery. Receivers must
// deduplicate by OutboxEvent.ID because a sink may succeed before ACK persists.
type DeliveryPolicy struct {
	BatchSize   int
	LeaseTTL    time.Duration
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	Now         func() time.Time
}

type DeliveryReport struct {
	Leased    int `json:"leased"`
	Delivered int `json:"delivered"`
	Deferred  int `json:"deferred"`
	Recovered int `json:"recovered"`
}

type DeliverySink func(context.Context, OutboxEvent) error

// DrainOutbox leases one page, delivers it, and records ACK or retry state.
// Delivery is at-least-once; the stable event ID is the receiver's dedupe key.
func DrainOutbox(ctx context.Context, store Store, owner execution.Identity, workerID, leaseID string, policy DeliveryPolicy, sink DeliverySink) (DeliveryReport, error) {
	if store == nil || sink == nil {
		return DeliveryReport{}, fmt.Errorf("outbox drain requires store and sink")
	}
	policy = normalizeDeliveryPolicy(policy)
	now := policy.Now()
	events, err := store.LeaseOutbox(ctx, owner, workerID, leaseID, policy.BatchSize, now, policy.LeaseTTL)
	if err != nil {
		return DeliveryReport{}, err
	}
	report := DeliveryReport{Leased: len(events)}
	var firstErr error
	for _, event := range events {
		if event.Recovery {
			report.Recovered++
		}
		deliveryErr := sink(ctx, event)
		settleCtx := context.WithoutCancel(ctx)
		settledAt := policy.Now()
		if deliveryErr == nil {
			_, ackErr := store.AckLeasedOutbox(settleCtx, owner, event.ID, event.Lease.ID, event.Attempts, settledAt)
			if ackErr == nil {
				report.Delivered++
				continue
			}
			deliveryErr = fmt.Errorf("ack continuation outbox event %s: %w", event.ID, ackErr)
		}
		next := settledAt.Add(deliveryBackoff(policy, event.Attempts))
		if _, retryErr := store.RetryOutbox(settleCtx, owner, event.ID, event.Lease.ID, event.Attempts, deliveryErr.Error(), next, settledAt); retryErr != nil {
			deliveryErr = errors.Join(deliveryErr, fmt.Errorf("defer continuation outbox event %s: %w", event.ID, retryErr))
		}
		report.Deferred++
		if firstErr == nil {
			firstErr = deliveryErr
		}
	}
	return report, firstErr
}

func normalizeDeliveryPolicy(in DeliveryPolicy) DeliveryPolicy {
	if in.BatchSize < 1 || in.BatchSize > 128 {
		in.BatchSize = 64
	}
	if in.LeaseTTL <= 0 {
		in.LeaseTTL = 30 * time.Second
	}
	if in.BaseBackoff <= 0 {
		in.BaseBackoff = time.Second
	}
	if in.MaxBackoff < in.BaseBackoff {
		in.MaxBackoff = time.Minute
	}
	if in.Now == nil {
		in.Now = time.Now
	}
	return in
}

func deliveryBackoff(policy DeliveryPolicy, attempt uint64) time.Duration {
	delay := policy.BaseBackoff
	for n := uint64(1); n < attempt && delay < policy.MaxBackoff; n++ {
		if delay > policy.MaxBackoff/2 {
			return policy.MaxBackoff
		}
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func outboxContentHash(event OutboxEvent) string {
	return hashSections(
		event.ID, event.Kind, event.Identity.PrincipalID, event.Identity.TenantID, event.Identity.SessionID,
		event.Identity.RunID, event.ContinuationID, event.CheckpointHash, string(event.ExpectedState),
		fmt.Sprint(event.ExpectedRevision), event.OperationKey, event.ReceiptHash, event.ClaimID,
		event.ReasonCode, event.At.UTC().Format(time.RFC3339Nano),
	)
}

func recoverCorruptOutboxEvent(event OutboxEvent) OutboxEvent {
	if event.OriginalContentHash == "" {
		event.OriginalContentHash = event.ContentHash
	}
	event.CorruptContentHash = outboxContentHash(event)
	event.Kind = "continuation.reconciliation_required"
	event.ReceiptHash = ""
	event.ClaimID = ""
	event.ReasonCode = "outbox.corrupt"
	event.Recovery = true
	event.ContentHash = outboxContentHash(event)
	return event
}

func cloneOutboxEvent(in OutboxEvent) OutboxEvent {
	if in.Lease != nil {
		lease := *in.Lease
		in.Lease = &lease
	}
	return in
}
