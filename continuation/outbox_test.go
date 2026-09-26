package continuation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

func TestOutboxLeaseRetryAckAndTombstone(t *testing.T) {
	store, owner, now := committedOutboxStore(t)
	leased, err := store.LeaseOutbox(context.Background(), owner, "worker-a", "lease-a", 10, now, time.Minute)
	if err != nil || len(leased) != 1 {
		t.Fatalf("leased=%+v err=%v", leased, err)
	}
	event := leased[0]
	if event.Status != OutboxLeased || event.Attempts != 1 || event.Lease == nil || event.ExpectedState != StateResultCommitted || event.ExpectedRevision != 2 || event.OperationKey == "" || event.ContentHash == "" {
		t.Fatalf("incomplete leased event=%+v", event)
	}
	if _, err := store.AckOutbox(context.Background(), owner, event.ID, now); !errors.Is(err, ErrOutboxLeaseConflict) {
		t.Fatalf("direct ACK ignored lease: %v", err)
	}
	next := now.Add(5 * time.Second)
	if _, err := store.RetryOutbox(context.Background(), owner, event.ID, event.Lease.ID, event.Attempts, "temporary", next, now); err != nil {
		t.Fatal(err)
	}
	if early, err := store.LeaseOutbox(context.Background(), owner, "worker-b", "lease-b", 10, next.Add(-time.Nanosecond), time.Minute); err != nil || len(early) != 0 {
		t.Fatalf("early=%+v err=%v", early, err)
	}
	leased, err = store.LeaseOutbox(context.Background(), owner, "worker-b", "lease-b", 10, next, time.Minute)
	if err != nil || len(leased) != 1 || leased[0].Attempts != 2 {
		t.Fatalf("re-leased=%+v err=%v", leased, err)
	}
	event = leased[0]
	if _, err := store.AckLeasedOutbox(context.Background(), owner, event.ID, event.Lease.ID, event.Attempts, next); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.PendingOutbox(context.Background(), owner, 10); err != nil || len(pending) != 0 {
		t.Fatalf("delivered event replayed: pending=%+v err=%v", pending, err)
	}
}

func TestOutboxExpiredLeaseIsReclaimedAfterCrash(t *testing.T) {
	store, owner, now := committedOutboxStore(t)
	first, err := store.LeaseOutbox(context.Background(), owner, "worker-a", "lease-a", 1, now, time.Second)
	if err != nil || len(first) != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := store.LeaseOutbox(context.Background(), owner, "worker-b", "lease-b", 1, now.Add(2*time.Second), time.Minute)
	if err != nil || len(second) != 1 || second[0].ID != first[0].ID || second[0].Attempts != 2 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if _, err := store.AckLeasedOutbox(context.Background(), owner, first[0].ID, first[0].Lease.ID, first[0].Attempts, now.Add(3*time.Second)); !errors.Is(err, ErrOutboxLeaseConflict) {
		t.Fatalf("stale worker ACK error=%v", err)
	}
}

func TestOutboxCorruptionBecomesSafeRecoveryEvent(t *testing.T) {
	store, owner, now := committedOutboxStore(t)
	key := outboxKey(owner, "event-result")
	store.mu.Lock()
	corrupt := store.outbox[key]
	corrupt.Kind = "continuation.completed"
	store.outbox[key] = corrupt
	store.mu.Unlock()

	leased, err := store.LeaseOutbox(context.Background(), owner, "worker", "lease", 1, now, time.Minute)
	if err != nil || len(leased) != 1 {
		t.Fatalf("leased=%+v err=%v", leased, err)
	}
	event := leased[0]
	if !event.Recovery || event.Kind != "continuation.reconciliation_required" || event.ReasonCode != "outbox.corrupt" || event.OriginalContentHash == "" || event.CorruptContentHash == "" || event.ContentHash != outboxContentHash(event) || event.ReceiptHash != "" || event.ClaimID != "" {
		t.Fatalf("unsafe recovery event=%+v", event)
	}
}

func TestDrainOutboxRetriesSinkFailure(t *testing.T) {
	store, owner, now := committedOutboxStore(t)
	clock := now
	policy := DeliveryPolicy{BatchSize: 1, LeaseTTL: time.Minute, BaseBackoff: time.Second, MaxBackoff: time.Minute, Now: func() time.Time { return clock }}
	calls := 0
	sink := func(context.Context, OutboxEvent) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("sink unavailable")
		}
		return nil
	}
	report, err := DrainOutbox(context.Background(), store, owner, "worker", "lease-a", policy, sink)
	if err == nil || report.Leased != 1 || report.Deferred != 1 || report.Delivered != 0 {
		t.Fatalf("first report=%+v err=%v", report, err)
	}
	clock = now.Add(time.Second)
	report, err = DrainOutbox(context.Background(), store, owner, "worker", "lease-b", policy, sink)
	if err != nil || report.Leased != 1 || report.Delivered != 1 || calls != 2 {
		t.Fatalf("second report=%+v calls=%d err=%v", report, calls, err)
	}
}

func committedOutboxStore(t *testing.T) (*MemoryStore, execution.Identity, time.Time) {
	t.Helper()
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	now := time.Unix(500, 0).UTC()
	record, _, err := store.Create(context.Background(), checkpoint, now.Add(-2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	receipt := testReceipt(t, checkpoint, EffectApplied)
	if _, err := store.CommitResult(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, receipt, "event-result", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	return store, checkpoint.Identity, now
}
