package continuation

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

// MemoryStore is a concurrency-safe reference implementation. It models the
// atomicity durable SQL/KV implementations must provide, but is not durable.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]Record
	outbox  map[string]OutboxEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]Record), outbox: make(map[string]OutboxEvent)}
}

func (s *MemoryStore) Create(ctx context.Context, checkpoint Checkpoint, at time.Time) (Record, bool, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, false, err
	}
	if err := validateSealedCheckpoint(checkpoint); err != nil {
		return Record{}, false, err
	}
	if at.IsZero() {
		return Record{}, false, fmt.Errorf("continuation create time is required")
	}
	key := recordKey(checkpoint.Identity, checkpoint.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[key]; ok {
		if existing.Checkpoint.ContentHash != checkpoint.ContentHash {
			return Record{}, false, ErrConflict
		}
		return cloneRecord(existing), false, nil
	}
	record := Record{Checkpoint: checkpoint, State: StateParked, Revision: 1, CreatedAt: at, UpdatedAt: at}
	s.records[key] = cloneRecord(record)
	return cloneRecord(record), true, nil
}

func (s *MemoryStore) CommitResult(ctx context.Context, owner execution.Identity, id string, expected uint64, receipt ResultReceipt, eventID string, at time.Time) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, key, err := s.owned(owner, id)
	if err != nil {
		return Record{}, err
	}
	if err := requireRevision(record, expected); err != nil {
		return Record{}, err
	}
	if record.State != StateParked {
		return Record{}, fmt.Errorf("commit continuation result from %s", record.State)
	}
	if err := validateSealedReceipt(record.Checkpoint, receipt); err != nil {
		return Record{}, err
	}
	next := StateResultCommitted
	kind := "continuation.result_committed"
	if receipt.EffectCertainty == EffectUnknown {
		next, kind = StateEffectUnknown, "continuation.effect_unknown"
	}
	record.State, record.Receipt, record.Revision, record.UpdatedAt = next, cloneReceipt(&receipt), record.Revision+1, at
	event, err := newOutboxEvent(record, eventID, kind, receipt.ReceiptHash, "", "", at)
	if err != nil {
		return Record{}, err
	}
	if err := s.appendOutbox(event); err != nil {
		return Record{}, err
	}
	s.records[key] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (s *MemoryStore) Claim(ctx context.Context, owner execution.Identity, id string, expected uint64, proof ResumeProof, claim Claim, eventID string, at time.Time) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, key, err := s.owned(owner, id)
	if err != nil {
		return Record{}, err
	}
	if err := requireRevision(record, expected); err != nil {
		return Record{}, err
	}
	if record.State != StateResultCommitted || record.Receipt == nil {
		return Record{}, ErrNotResumable
	}
	if err := validateResumeProof(record, proof); err != nil {
		return Record{}, err
	}
	if claim.ID == "" || claim.WorkerID == "" || !claim.ExpiresAt.After(at) {
		return Record{}, fmt.Errorf("continuation claim requires ID, worker, and a future expiry")
	}
	record.State, record.Proof, record.Claim = StateClaimed, cloneProof(&proof), cloneClaim(&claim)
	record.Revision, record.UpdatedAt = record.Revision+1, at
	event, err := newOutboxEvent(record, eventID, "continuation.claimed", record.Receipt.ReceiptHash, claim.ID, "", at)
	if err != nil {
		return Record{}, err
	}
	if err := s.appendOutbox(event); err != nil {
		return Record{}, err
	}
	s.records[key] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (s *MemoryStore) ExpireClaim(ctx context.Context, owner execution.Identity, id string, expected uint64, eventID string, at time.Time) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, key, err := s.owned(owner, id)
	if err != nil {
		return Record{}, err
	}
	if err := requireRevision(record, expected); err != nil {
		return Record{}, err
	}
	if record.State != StateClaimed || record.Claim == nil || at.Before(record.Claim.ExpiresAt) {
		return Record{}, fmt.Errorf("continuation claim is not expired")
	}
	claimID := record.Claim.ID
	record.State, record.ReasonCode, record.Claim = StateReconcileRequired, "claim.expired", nil
	record.Revision, record.UpdatedAt = record.Revision+1, at
	event, err := newOutboxEvent(record, eventID, "continuation.reconcile_required", receiptHashOf(record), claimID, record.ReasonCode, at)
	if err != nil {
		return Record{}, err
	}
	if err := s.appendOutbox(event); err != nil {
		return Record{}, err
	}
	s.records[key] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (s *MemoryStore) Settle(ctx context.Context, owner execution.Identity, id string, expected uint64, next State, reason, eventID string, at time.Time) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if next != StateCompleted && next != StateFailed && next != StateEffectUnknown {
		return Record{}, fmt.Errorf("invalid continuation settlement %q", next)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, key, err := s.owned(owner, id)
	if err != nil {
		return Record{}, err
	}
	if err := requireRevision(record, expected); err != nil {
		return Record{}, err
	}
	if record.State != StateClaimed || record.Claim == nil {
		return Record{}, fmt.Errorf("settle continuation from %s", record.State)
	}
	claimID := record.Claim.ID
	record.State, record.ReasonCode, record.Claim = next, reason, nil
	record.Revision, record.UpdatedAt = record.Revision+1, at
	event, err := newOutboxEvent(record, eventID, "continuation."+string(next), receiptHashOf(record), claimID, reason, at)
	if err != nil {
		return Record{}, err
	}
	if err := s.appendOutbox(event); err != nil {
		return Record{}, err
	}
	s.records[key] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (s *MemoryStore) Reconcile(ctx context.Context, owner execution.Identity, id string, expected uint64, next State, receipt *ResultReceipt, reason, eventID string, at time.Time) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if next != StateResultCommitted && next != StateFailed {
		return Record{}, fmt.Errorf("invalid reconciliation settlement %q", next)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, key, err := s.owned(owner, id)
	if err != nil {
		return Record{}, err
	}
	if err := requireRevision(record, expected); err != nil {
		return Record{}, err
	}
	if record.State != StateEffectUnknown && record.State != StateReconcileRequired {
		return Record{}, fmt.Errorf("reconcile continuation from %s", record.State)
	}
	if next == StateResultCommitted {
		if receipt == nil {
			return Record{}, fmt.Errorf("resumable reconciliation requires an exact result receipt")
		}
		if err := validateSealedReceipt(record.Checkpoint, *receipt); err != nil {
			return Record{}, err
		}
		if receipt.EffectCertainty == EffectUnknown {
			return Record{}, fmt.Errorf("reconciliation receipt remains effect-unknown")
		}
		record.Receipt = cloneReceipt(receipt)
	} else if receipt != nil {
		return Record{}, fmt.Errorf("failed reconciliation cannot replace the result receipt")
	}
	record.State, record.ReasonCode, record.Proof, record.Claim = next, reason, nil, nil
	record.Revision, record.UpdatedAt = record.Revision+1, at
	event, err := newOutboxEvent(record, eventID, "continuation.reconciled", receiptHashOf(record), "", reason, at)
	if err != nil {
		return Record{}, err
	}
	if err := s.appendOutbox(event); err != nil {
		return Record{}, err
	}
	s.records[key] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (s *MemoryStore) Get(ctx context.Context, owner execution.Identity, id string) (Record, bool, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[recordKey(owner, id)]
	if !ok {
		return Record{}, false, nil
	}
	if record.Checkpoint.Identity != owner {
		return Record{}, false, ErrOwnerMismatch
	}
	return cloneRecord(record), true, nil
}

func (s *MemoryStore) Pending(ctx context.Context, owner execution.Identity, limit int) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("continuation pending limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, limit)
	for _, record := range s.records {
		if record.Checkpoint.Identity == owner && record.State != StateCompleted && record.State != StateFailed {
			out = append(out, cloneRecord(record))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) PendingOutbox(ctx context.Context, owner execution.Identity, limit int) ([]OutboxEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("continuation outbox limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]OutboxEvent, 0, limit)
	for _, event := range s.outbox {
		if event.Identity == owner && event.Status != OutboxDelivered {
			out = append(out, cloneOutboxEvent(event))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ID < out[j].ID
		}
		return out[i].At.Before(out[j].At)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) LeaseOutbox(ctx context.Context, owner execution.Identity, workerID, leaseID string, limit int, now time.Time, ttl time.Duration) ([]OutboxEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if workerID == "" || leaseID == "" || limit <= 0 || limit > 128 || now.IsZero() || ttl <= 0 {
		return nil, fmt.Errorf("outbox lease requires worker, lease ID, bounded limit, time, and positive TTL")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	eligible := make([]string, 0, limit)
	for key, event := range s.outbox {
		if event.Identity != owner || event.Status == OutboxDelivered || (!event.NextAttemptAt.IsZero() && now.Before(event.NextAttemptAt)) {
			continue
		}
		if event.Status == OutboxLeased && event.Lease != nil && now.Before(event.Lease.ExpiresAt) {
			continue
		}
		eligible = append(eligible, key)
	}
	sort.Slice(eligible, func(i, j int) bool {
		left, right := s.outbox[eligible[i]], s.outbox[eligible[j]]
		if left.At.Equal(right.At) {
			return left.ID < right.ID
		}
		return left.At.Before(right.At)
	})
	if len(eligible) > limit {
		eligible = eligible[:limit]
	}
	out := make([]OutboxEvent, 0, len(eligible))
	for _, key := range eligible {
		event := s.outbox[key]
		if event.ContentHash == "" || event.ContentHash != outboxContentHash(event) {
			event = recoverCorruptOutboxEvent(event)
		}
		event.Status = OutboxLeased
		event.Attempts++
		event.Lease = &OutboxLease{ID: leaseID + ":" + event.ID, WorkerID: workerID, ExpiresAt: now.Add(ttl)}
		s.outbox[key] = cloneOutboxEvent(event)
		out = append(out, cloneOutboxEvent(event))
	}
	return out, nil
}

func (s *MemoryStore) RetryOutbox(ctx context.Context, owner execution.Identity, eventID, leaseID string, expectedAttempt uint64, reason string, next, at time.Time) (OutboxEvent, error) {
	if err := ctx.Err(); err != nil {
		return OutboxEvent{}, err
	}
	if reason == "" || next.Before(at) || at.IsZero() {
		return OutboxEvent{}, fmt.Errorf("outbox retry requires reason and valid retry time")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	event, key, err := s.leasedOutbox(owner, eventID, leaseID, expectedAttempt)
	if err != nil {
		return OutboxEvent{}, err
	}
	event.Status, event.Lease = OutboxPending, nil
	event.LastError, event.NextAttemptAt = reason, next
	s.outbox[key] = cloneOutboxEvent(event)
	return cloneOutboxEvent(event), nil
}

func (s *MemoryStore) AckLeasedOutbox(ctx context.Context, owner execution.Identity, eventID, leaseID string, expectedAttempt uint64, at time.Time) (OutboxEvent, error) {
	if err := ctx.Err(); err != nil {
		return OutboxEvent{}, err
	}
	if at.IsZero() {
		return OutboxEvent{}, fmt.Errorf("outbox acknowledgement time is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	event, key, err := s.leasedOutbox(owner, eventID, leaseID, expectedAttempt)
	if err != nil {
		return OutboxEvent{}, err
	}
	event.Status, event.Lease, event.DeliveredAt = OutboxDelivered, nil, at
	event.LastError, event.NextAttemptAt = "", time.Time{}
	s.outbox[key] = cloneOutboxEvent(event)
	return cloneOutboxEvent(event), nil
}

func (s *MemoryStore) AckOutbox(ctx context.Context, owner execution.Identity, eventID string, at time.Time) (OutboxEvent, error) {
	if err := ctx.Err(); err != nil {
		return OutboxEvent{}, err
	}
	if at.IsZero() {
		return OutboxEvent{}, fmt.Errorf("outbox acknowledgement time is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := outboxKey(owner, eventID)
	event, ok := s.outbox[key]
	if !ok {
		return OutboxEvent{}, fmt.Errorf("continuation outbox event %q not found", eventID)
	}
	if event.Identity != owner {
		return OutboxEvent{}, ErrOwnerMismatch
	}
	if event.Status == OutboxLeased {
		return OutboxEvent{}, ErrOutboxLeaseConflict
	}
	if event.Status != OutboxDelivered {
		event.Status, event.DeliveredAt = OutboxDelivered, at
		s.outbox[key] = event
	}
	return cloneOutboxEvent(event), nil
}

func (s *MemoryStore) leasedOutbox(owner execution.Identity, eventID, leaseID string, expectedAttempt uint64) (OutboxEvent, string, error) {
	key := outboxKey(owner, eventID)
	event, ok := s.outbox[key]
	if !ok {
		return OutboxEvent{}, "", fmt.Errorf("continuation outbox event %q not found", eventID)
	}
	if event.Identity != owner {
		return OutboxEvent{}, "", ErrOwnerMismatch
	}
	if event.Status != OutboxLeased || event.Lease == nil || event.Lease.ID != leaseID {
		return OutboxEvent{}, "", ErrOutboxLeaseConflict
	}
	if event.Attempts != expectedAttempt {
		return OutboxEvent{}, "", ErrOutboxAttempt
	}
	return event, key, nil
}

func (s *MemoryStore) owned(owner execution.Identity, id string) (Record, string, error) {
	key := recordKey(owner, id)
	record, ok := s.records[key]
	if !ok {
		return Record{}, "", fmt.Errorf("continuation %q not found", id)
	}
	if record.Checkpoint.Identity != owner {
		return Record{}, "", ErrOwnerMismatch
	}
	return record, key, nil
}

func (s *MemoryStore) appendOutbox(event OutboxEvent) error {
	key := outboxKey(event.Identity, event.ID)
	if existing, ok := s.outbox[key]; ok {
		if existing.Kind != event.Kind || existing.ContinuationID != event.ContinuationID ||
			existing.ExpectedState != event.ExpectedState || existing.ExpectedRevision != event.ExpectedRevision ||
			existing.OperationKey != event.OperationKey || existing.ContentHash != event.ContentHash {
			return fmt.Errorf("continuation outbox event %q reused with conflicting payload", event.ID)
		}
		return nil
	}
	s.outbox[key] = event
	return nil
}

func validateSealedReceipt(checkpoint Checkpoint, receipt ResultReceipt) error {
	if err := validateReceiptBinding(checkpoint, receipt); err != nil {
		return err
	}
	if receipt.ReceiptHash == "" || receipt.ReceiptHash != receiptHash(receipt) {
		return fmt.Errorf("tool result receipt hash mismatch")
	}
	return nil
}

func validateResumeProof(record Record, proof ResumeProof) error {
	receipt := record.Receipt
	if receipt == nil || proof.CheckpointHash != record.Checkpoint.ContentHash || proof.ReceiptHash != receipt.ReceiptHash ||
		proof.Runtime != record.Checkpoint.Runtime || proof.Projection != receipt.Projection ||
		proof.ResultEventID != receipt.ResultEventID || proof.ResultEventHash != receipt.ResultEventHash ||
		proof.Context.ID != record.Checkpoint.Context.ID || proof.Context.Epoch != record.Checkpoint.Context.Epoch ||
		proof.Context.Revision <= record.Checkpoint.Context.Revision || proof.Context.Digest == "" ||
		proof.Context.Digest == record.Checkpoint.Context.Digest || proof.TrajectoryCursor < record.Checkpoint.TrajectoryCursor {
		return ErrProofMismatch
	}
	return nil
}

func requireRevision(record Record, expected uint64) error {
	if record.Revision != expected {
		return ErrRevisionConflict
	}
	return nil
}

func newOutboxEvent(record Record, id, kind, receiptHash, claimID, reason string, at time.Time) (OutboxEvent, error) {
	if id == "" || kind == "" || at.IsZero() {
		return OutboxEvent{}, fmt.Errorf("continuation transition requires outbox ID, kind, and time")
	}
	event := OutboxEvent{
		ID: id, Kind: kind, Identity: record.Checkpoint.Identity, ContinuationID: record.Checkpoint.ID,
		CheckpointHash: record.Checkpoint.ContentHash, ExpectedState: record.State, ExpectedRevision: record.Revision,
		OperationKey: hashSections(record.Checkpoint.ContentHash, string(record.State), fmt.Sprint(record.Revision), kind, receiptHash, claimID, reason),
		ReceiptHash:  receiptHash, ClaimID: claimID, ReasonCode: reason, Status: OutboxPending, At: at,
	}
	event.ContentHash = outboxContentHash(event)
	return event, nil
}

func receiptHashOf(record Record) string {
	if record.Receipt == nil {
		return ""
	}
	return record.Receipt.ReceiptHash
}

func recordKey(owner execution.Identity, id string) string {
	return owner.PrincipalID + "\x00" + owner.TenantID + "\x00" + owner.SessionID + "\x00" + owner.RunID + "\x00" + id
}

func outboxKey(owner execution.Identity, id string) string { return recordKey(owner, id) }

func cloneRecord(in Record) Record {
	in.Receipt = cloneReceipt(in.Receipt)
	in.Proof = cloneProof(in.Proof)
	in.Claim = cloneClaim(in.Claim)
	return in
}

func cloneReceipt(in *ResultReceipt) *ResultReceipt {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneProof(in *ResumeProof) *ResumeProof {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneClaim(in *Claim) *Claim {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
