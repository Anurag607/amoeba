package continuation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

func TestContinuationExactResumeAndAtomicOutbox(t *testing.T) {
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	owner := checkpoint.Identity
	now := time.Unix(100, 0).UTC()
	record, fresh, err := store.Create(context.Background(), checkpoint, now)
	if err != nil || !fresh || record.State != StateParked {
		t.Fatalf("create record=%+v fresh=%v err=%v", record, fresh, err)
	}
	if retry, fresh, err := store.Create(context.Background(), checkpoint, now); err != nil || fresh || retry.Revision != record.Revision {
		t.Fatalf("exact retry record=%+v fresh=%v err=%v", retry, fresh, err)
	}

	receipt := testReceipt(t, checkpoint, EffectApplied)
	record, err = store.CommitResult(context.Background(), owner, checkpoint.ID, record.Revision, receipt, "event-result", now.Add(time.Second))
	if err != nil || record.State != StateResultCommitted || record.Receipt == nil {
		t.Fatalf("commit result record=%+v err=%v", record, err)
	}
	proof := testProof(checkpoint, receipt)
	claim := Claim{ID: "claim-1", WorkerID: "worker-1", ExpiresAt: now.Add(time.Minute)}
	record, err = store.Claim(context.Background(), owner, checkpoint.ID, record.Revision, proof, claim, "event-claim", now.Add(2*time.Second))
	if err != nil || record.State != StateClaimed || record.Claim == nil {
		t.Fatalf("claim record=%+v err=%v", record, err)
	}
	if _, err := store.Settle(context.Background(), owner, checkpoint.ID, record.Revision-1, StateCompleted, "", "event-stale", now.Add(3*time.Second)); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale settle error=%v", err)
	}
	record, err = store.Settle(context.Background(), owner, checkpoint.ID, record.Revision, StateCompleted, "", "event-complete", now.Add(3*time.Second))
	if err != nil || record.State != StateCompleted || record.Claim != nil {
		t.Fatalf("settle record=%+v err=%v", record, err)
	}
	events, err := store.PendingOutbox(context.Background(), owner, 10)
	if err != nil || len(events) != 3 || events[0].ID != "event-result" || events[1].ID != "event-claim" || events[2].ID != "event-complete" {
		t.Fatalf("outbox events=%+v err=%v", events, err)
	}
	acked, err := store.AckOutbox(context.Background(), owner, events[0].ID, now.Add(4*time.Second))
	if err != nil || acked.DeliveredAt.IsZero() {
		t.Fatalf("ack event=%+v err=%v", acked, err)
	}
	events, _ = store.PendingOutbox(context.Background(), owner, 10)
	if len(events) != 2 {
		t.Fatalf("pending outbox after ACK=%+v", events)
	}
}

func TestContinuationRejectsMismatchedResumeProof(t *testing.T) {
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	now := time.Unix(200, 0).UTC()
	record, _, _ := store.Create(context.Background(), checkpoint, now)
	receipt := testReceipt(t, checkpoint, EffectNoEffect)
	record, _ = store.CommitResult(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, receipt, "event-result", now.Add(time.Second))
	proof := testProof(checkpoint, receipt)
	proof.ResultEventHash = "sha256:tampered"
	_, err := store.Claim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, proof, Claim{ID: "claim", WorkerID: "worker", ExpiresAt: now.Add(time.Minute)}, "event-claim", now.Add(2*time.Second))
	if !errors.Is(err, ErrProofMismatch) {
		t.Fatalf("claim error=%v", err)
	}
	stored, _, _ := store.Get(context.Background(), checkpoint.Identity, checkpoint.ID)
	events, _ := store.PendingOutbox(context.Background(), checkpoint.Identity, 10)
	if stored.State != StateResultCommitted || len(events) != 1 {
		t.Fatalf("failed proof mutated state=%s events=%+v", stored.State, events)
	}
}

func TestContinuationEffectUnknownRequiresReconciliation(t *testing.T) {
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	now := time.Unix(300, 0).UTC()
	record, _, _ := store.Create(context.Background(), checkpoint, now)
	unknown := testReceipt(t, checkpoint, EffectUnknown)
	record, err := store.CommitResult(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, unknown, "event-unknown", now.Add(time.Second))
	if err != nil || record.State != StateEffectUnknown {
		t.Fatalf("unknown record=%+v err=%v", record, err)
	}
	_, err = store.Claim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, testProof(checkpoint, unknown), Claim{ID: "claim", WorkerID: "worker", ExpiresAt: now.Add(time.Minute)}, "event-claim", now.Add(2*time.Second))
	if !errors.Is(err, ErrNotResumable) {
		t.Fatalf("effect-unknown claim error=%v", err)
	}
	confirmed := testReceipt(t, checkpoint, EffectApplied)
	record, err = store.Reconcile(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, StateResultCommitted, &confirmed, "effect.confirmed", "event-reconciled", now.Add(3*time.Second))
	if err != nil || record.State != StateResultCommitted || record.Receipt.ReceiptHash != confirmed.ReceiptHash {
		t.Fatalf("reconcile record=%+v err=%v", record, err)
	}
}

func TestExpiredClaimRequiresReconciliation(t *testing.T) {
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	now := time.Unix(400, 0).UTC()
	record, _, _ := store.Create(context.Background(), checkpoint, now)
	receipt := testReceipt(t, checkpoint, EffectApplied)
	record, _ = store.CommitResult(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, receipt, "event-result", now.Add(time.Second))
	record, _ = store.Claim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, testProof(checkpoint, receipt), Claim{ID: "claim", WorkerID: "worker", ExpiresAt: now.Add(3 * time.Second)}, "event-claim", now.Add(2*time.Second))
	if _, err := store.ExpireClaim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, "event-early", now.Add(2*time.Second)); err == nil {
		t.Fatal("unexpired claim was expired")
	}
	record, err := store.ExpireClaim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, "event-expired", now.Add(4*time.Second))
	if err != nil || record.State != StateReconcileRequired {
		t.Fatalf("expire record=%+v err=%v", record, err)
	}
}

func testCheckpoint(t *testing.T) Checkpoint {
	t.Helper()
	checkpoint, err := SealCheckpoint(Checkpoint{
		ID: "continuation-1", Identity: execution.Identity{PrincipalID: "owner", SessionID: "session", RunID: "run"}, InputID: "input",
		Runtime:               RuntimeBinding{Harness: "claude-code", HarnessVersion: "1", HarnessConfigDigest: "sha256:harness", ResumeCapability: "exact_checkpoint", Model: "model", EnvironmentDigest: "sha256:env", PolicyVersion: "policy-v1", CatalogVersion: "catalog-v1"},
		Context:               ContextBinding{ID: "context", Revision: 4, Digest: "sha256:context-before", Epoch: 2},
		Tool:                  ToolBinding{Name: "write_file", Version: "1", RoundID: "round", CallID: "call", AttemptID: "attempt", ApprovalID: "approval", EnvelopeHash: "sha256:envelope", TargetHash: "sha256:target", IdempotencyKey: "idem"},
		DelegationLeaseDigest: "sha256:lease", TrajectoryCursor: 7, ResumeTokenHash: "sha256:token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func testReceipt(t *testing.T, checkpoint Checkpoint, certainty EffectCertainty) ResultReceipt {
	t.Helper()
	receipt, err := SealResultReceipt(checkpoint, ResultReceipt{
		ContinuationID: checkpoint.ID, CheckpointHash: checkpoint.ContentHash, AttemptID: checkpoint.Tool.AttemptID,
		RoundID: checkpoint.Tool.RoundID, CallID: checkpoint.Tool.CallID, ToolName: checkpoint.Tool.Name,
		ResultEventID: "result-event", ResultEventHash: "sha256:event", SemanticResultHash: "sha256:semantic",
		ObservableResultHash: "sha256:observable", EffectCertainty: certainty,
		Projection: ProjectionBinding{ID: "projection", LogicalCallID: "logical-call", ToolName: checkpoint.Tool.Name},
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func testProof(checkpoint Checkpoint, receipt ResultReceipt) ResumeProof {
	return ResumeProof{
		CheckpointHash: checkpoint.ContentHash, ReceiptHash: receipt.ReceiptHash, Runtime: checkpoint.Runtime,
		Context:    ContextBinding{ID: checkpoint.Context.ID, Revision: checkpoint.Context.Revision + 1, Digest: "sha256:context-after", Epoch: checkpoint.Context.Epoch},
		Projection: receipt.Projection, ResultEventID: receipt.ResultEventID, ResultEventHash: receipt.ResultEventHash,
		TrajectoryCursor: checkpoint.TrajectoryCursor + 1,
	}
}
