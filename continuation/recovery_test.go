package continuation

import (
	"context"
	"errors"
	"testing"
	"time"
)

type staticEvidenceReader struct {
	evidence ResumeEvidence
}

func (r staticEvidenceReader) ObserveResume(context.Context, Checkpoint, ResultReceipt) (ResumeEvidence, error) {
	return r.evidence, nil
}

func TestRecoveryCoordinatorBuildsProofFromAuthoritativeEvidence(t *testing.T) {
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	now := time.Unix(600, 0).UTC()
	record, _, err := store.Create(context.Background(), checkpoint, now)
	if err != nil {
		t.Fatal(err)
	}
	receipt := testReceipt(t, checkpoint, EffectNoEffect)
	record, err = store.CommitResult(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, receipt, "event-result", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	proof := testProof(checkpoint, receipt)
	coordinator := RecoveryCoordinator{Store: store, Evidence: staticEvidenceReader{evidence: ResumeEvidence{
		Runtime: proof.Runtime, Context: proof.Context, Projection: proof.Projection, ResultEventID: proof.ResultEventID,
		ResultEventHash: proof.ResultEventHash, TrajectoryCursor: proof.TrajectoryCursor,
	}}}
	claim := Claim{ID: "claim", WorkerID: "worker", ExpiresAt: now.Add(time.Minute)}
	claimed, err := coordinator.Claim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, claim, "event-claim", now.Add(2*time.Second))
	if err != nil || claimed.State != StateClaimed || claimed.Proof == nil || claimed.Proof.ReceiptHash != receipt.ReceiptHash {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}
}

func TestRecoveryCoordinatorRejectsStaleAuthoritativeEvidence(t *testing.T) {
	store := NewMemoryStore()
	checkpoint := testCheckpoint(t)
	now := time.Unix(700, 0).UTC()
	record, _, _ := store.Create(context.Background(), checkpoint, now)
	receipt := testReceipt(t, checkpoint, EffectApplied)
	record, _ = store.CommitResult(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, receipt, "event-result", now.Add(time.Second))
	proof := testProof(checkpoint, receipt)
	proof.Context.Revision = checkpoint.Context.Revision
	coordinator := RecoveryCoordinator{Store: store, Evidence: staticEvidenceReader{evidence: ResumeEvidence{
		Runtime: proof.Runtime, Context: proof.Context, Projection: proof.Projection, ResultEventID: proof.ResultEventID,
		ResultEventHash: proof.ResultEventHash, TrajectoryCursor: proof.TrajectoryCursor,
	}}}
	_, err := coordinator.Claim(context.Background(), checkpoint.Identity, checkpoint.ID, record.Revision, Claim{ID: "claim", WorkerID: "worker", ExpiresAt: now.Add(time.Minute)}, "event-claim", now.Add(2*time.Second))
	if !errors.Is(err, ErrProofMismatch) {
		t.Fatalf("claim error=%v", err)
	}
	stored, _, _ := store.Get(context.Background(), checkpoint.Identity, checkpoint.ID)
	if stored.State != StateResultCommitted {
		t.Fatalf("stale evidence mutated state=%s", stored.State)
	}
}
