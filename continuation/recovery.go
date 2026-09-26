package continuation

import (
	"context"
	"fmt"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

// ResumeEvidence is loaded from authoritative host stores. It deliberately
// excludes caller-authored checkpoint and receipt hashes.
type ResumeEvidence struct {
	Runtime          RuntimeBinding    `json:"runtime"`
	Context          ContextBinding    `json:"context"`
	Projection       ProjectionBinding `json:"projection"`
	ResultEventID    string            `json:"result_event_id"`
	ResultEventHash  string            `json:"result_event_hash"`
	TrajectoryCursor uint64            `json:"trajectory_cursor"`
}

// EvidenceReader joins the host's runtime, context, archive, and trajectory
// stores into one observation. Implementations must not trust model or client
// fields when constructing evidence.
type EvidenceReader interface {
	ObserveResume(context.Context, Checkpoint, ResultReceipt) (ResumeEvidence, error)
}

// RecoveryCoordinator is the preferred claim boundary. It reconstructs proof
// from authoritative evidence immediately before the store's CAS transition.
type RecoveryCoordinator struct {
	Store    Store
	Evidence EvidenceReader
}

func (c RecoveryCoordinator) Claim(ctx context.Context, owner execution.Identity, continuationID string, expected uint64, claim Claim, eventID string, at time.Time) (Record, error) {
	if c.Store == nil || c.Evidence == nil {
		return Record{}, fmt.Errorf("recovery coordinator requires store and evidence reader")
	}
	record, found, err := c.Store.Get(ctx, owner, continuationID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, fmt.Errorf("continuation %q not found", continuationID)
	}
	if record.Revision != expected {
		return Record{}, ErrRevisionConflict
	}
	if record.State != StateResultCommitted || record.Receipt == nil {
		return Record{}, ErrNotResumable
	}
	evidence, err := c.Evidence.ObserveResume(ctx, record.Checkpoint, *record.Receipt)
	if err != nil {
		return Record{}, fmt.Errorf("observe continuation resume evidence: %w", err)
	}
	proof := ResumeProof{
		CheckpointHash: record.Checkpoint.ContentHash, ReceiptHash: record.Receipt.ReceiptHash,
		Runtime: evidence.Runtime, Context: evidence.Context, Projection: evidence.Projection,
		ResultEventID: evidence.ResultEventID, ResultEventHash: evidence.ResultEventHash,
		TrajectoryCursor: evidence.TrajectoryCursor,
	}
	return c.Store.Claim(ctx, owner, continuationID, expected, proof, claim, eventID, at)
}
