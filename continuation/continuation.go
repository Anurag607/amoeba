// Package continuation defines crash-safe, exact resume contracts for parked
// agent work. Durable implementations must commit each state transition and
// its outbox event in one storage transaction.
package continuation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

const (
	CheckpointVersion = 1
	ReceiptVersion    = 1
)

type State string

const (
	StateParked            State = "parked"
	StateResultCommitted   State = "result_committed"
	StateClaimed           State = "claimed"
	StateCompleted         State = "completed"
	StateFailed            State = "failed"
	StateEffectUnknown     State = "effect_unknown"
	StateReconcileRequired State = "reconcile_required"
)

type EffectCertainty string

const (
	EffectNoEffect EffectCertainty = "no_effect"
	EffectApplied  EffectCertainty = "effect_applied"
	EffectUnknown  EffectCertainty = "effect_unknown"
)

// RuntimeBinding prevents a continuation from silently changing the harness,
// model, environment, policy, or tool catalog that created its checkpoint.
type RuntimeBinding struct {
	Harness             string `json:"harness"`
	HarnessVersion      string `json:"harness_version"`
	HarnessConfigDigest string `json:"harness_config_digest"`
	ResumeCapability    string `json:"resume_capability"`
	Model               string `json:"model"`
	EnvironmentDigest   string `json:"environment_digest"`
	PolicyVersion       string `json:"policy_version"`
	CatalogVersion      string `json:"catalog_version"`
}

// ContextBinding identifies one immutable context projection head.
type ContextBinding struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
	Epoch    uint64 `json:"epoch"`
}

// ToolBinding identifies the exact approved operation parked by a checkpoint.
type ToolBinding struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	RoundID        string `json:"round_id"`
	CallID         string `json:"call_id"`
	AttemptID      string `json:"attempt_id"`
	ApprovalID     string `json:"approval_id"`
	EnvelopeHash   string `json:"envelope_hash"`
	TargetHash     string `json:"target_hash"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Checkpoint is immutable resume intent. ResumeTokenHash binds a secret token
// without requiring the continuation store to retain the token itself.
type Checkpoint struct {
	Version               int                `json:"version"`
	ID                    string             `json:"id"`
	Identity              execution.Identity `json:"identity"`
	InputID               string             `json:"input_id"`
	Runtime               RuntimeBinding     `json:"runtime"`
	Context               ContextBinding     `json:"context"`
	Tool                  ToolBinding        `json:"tool"`
	DelegationLeaseDigest string             `json:"delegation_lease_digest,omitempty"`
	TrajectoryCursor      uint64             `json:"trajectory_cursor"`
	ResumeTokenHash       string             `json:"resume_token_hash,omitempty"`
	ContentHash           string             `json:"content_hash"`
}

// ProjectionBinding proves where the exact tool exchange was incorporated.
type ProjectionBinding struct {
	ID            string `json:"id"`
	LogicalCallID string `json:"logical_call_id"`
	ToolName      string `json:"tool_name"`
}

// ResultReceipt couples the semantic tool outcome to its exact durable event
// and the context projection that must cover it before inference resumes.
type ResultReceipt struct {
	Version              int               `json:"version"`
	ContinuationID       string            `json:"continuation_id"`
	CheckpointHash       string            `json:"checkpoint_hash"`
	AttemptID            string            `json:"attempt_id"`
	RoundID              string            `json:"round_id"`
	CallID               string            `json:"call_id"`
	ToolName             string            `json:"tool_name"`
	ResultEventID        string            `json:"result_event_id"`
	ResultEventHash      string            `json:"result_event_hash"`
	SemanticResultHash   string            `json:"semantic_result_hash"`
	ObservableResultHash string            `json:"observable_result_hash"`
	IsError              bool              `json:"is_error"`
	EffectCertainty      EffectCertainty   `json:"effect_certainty"`
	Projection           ProjectionBinding `json:"projection"`
	ReceiptHash          string            `json:"receipt_hash"`
}

// ResumeProof is reconstructed from authoritative stores immediately before
// a claim. The context revision must be newer than the parked baseline.
type ResumeProof struct {
	CheckpointHash   string            `json:"checkpoint_hash"`
	ReceiptHash      string            `json:"receipt_hash"`
	Runtime          RuntimeBinding    `json:"runtime"`
	Context          ContextBinding    `json:"context"`
	Projection       ProjectionBinding `json:"projection"`
	ResultEventID    string            `json:"result_event_id"`
	ResultEventHash  string            `json:"result_event_hash"`
	TrajectoryCursor uint64            `json:"trajectory_cursor"`
}

type Claim struct {
	ID        string    `json:"id"`
	WorkerID  string    `json:"worker_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// OutboxEvent is intentionally metadata-only. Prompts, credentials, resume
// tokens, and raw tool output never belong in the transactional outbox.
type OutboxEvent struct {
	ID                  string             `json:"id"`
	Kind                string             `json:"kind"`
	Identity            execution.Identity `json:"identity"`
	ContinuationID      string             `json:"continuation_id"`
	CheckpointHash      string             `json:"checkpoint_hash"`
	ExpectedState       State              `json:"expected_state"`
	ExpectedRevision    uint64             `json:"expected_revision"`
	OperationKey        string             `json:"operation_key"`
	ReceiptHash         string             `json:"receipt_hash,omitempty"`
	ClaimID             string             `json:"claim_id,omitempty"`
	ReasonCode          string             `json:"reason_code,omitempty"`
	ContentHash         string             `json:"content_hash"`
	OriginalContentHash string             `json:"original_content_hash,omitempty"`
	CorruptContentHash  string             `json:"corrupt_content_hash,omitempty"`
	Status              OutboxStatus       `json:"status"`
	Recovery            bool               `json:"recovery,omitempty"`
	Attempts            uint64             `json:"attempts"`
	NextAttemptAt       time.Time          `json:"next_attempt_at,omitempty"`
	Lease               *OutboxLease       `json:"lease,omitempty"`
	LastError           string             `json:"last_error,omitempty"`
	At                  time.Time          `json:"at"`
	DeliveredAt         time.Time          `json:"delivered_at,omitempty"`
}

type Record struct {
	Checkpoint Checkpoint     `json:"checkpoint"`
	State      State          `json:"state"`
	Receipt    *ResultReceipt `json:"receipt,omitempty"`
	Proof      *ResumeProof   `json:"proof,omitempty"`
	Claim      *Claim         `json:"claim,omitempty"`
	ReasonCode string         `json:"reason_code,omitempty"`
	Revision   uint64         `json:"revision"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

var (
	ErrConflict         = errors.New("continuation identity reused with different checkpoint")
	ErrRevisionConflict = errors.New("continuation revision conflict")
	ErrOwnerMismatch    = errors.New("continuation owner mismatch")
	ErrNotResumable     = errors.New("continuation is not resumable")
	ErrProofMismatch    = errors.New("continuation resume proof mismatch")
)

// Store is the durable continuation and transactional-outbox boundary.
// Mutation methods must update the record and append their event atomically.
type Store interface {
	Create(context.Context, Checkpoint, time.Time) (Record, bool, error)
	CommitResult(context.Context, execution.Identity, string, uint64, ResultReceipt, string, time.Time) (Record, error)
	Claim(context.Context, execution.Identity, string, uint64, ResumeProof, Claim, string, time.Time) (Record, error)
	ExpireClaim(context.Context, execution.Identity, string, uint64, string, time.Time) (Record, error)
	Settle(context.Context, execution.Identity, string, uint64, State, string, string, time.Time) (Record, error)
	Reconcile(context.Context, execution.Identity, string, uint64, State, *ResultReceipt, string, string, time.Time) (Record, error)
	Get(context.Context, execution.Identity, string) (Record, bool, error)
	Pending(context.Context, execution.Identity, int) ([]Record, error)
	PendingOutbox(context.Context, execution.Identity, int) ([]OutboxEvent, error)
	LeaseOutbox(context.Context, execution.Identity, string, string, int, time.Time, time.Duration) ([]OutboxEvent, error)
	RetryOutbox(context.Context, execution.Identity, string, string, uint64, string, time.Time, time.Time) (OutboxEvent, error)
	AckLeasedOutbox(context.Context, execution.Identity, string, string, uint64, time.Time) (OutboxEvent, error)
	AckOutbox(context.Context, execution.Identity, string, time.Time) (OutboxEvent, error)
}

func SealCheckpoint(in Checkpoint) (Checkpoint, error) {
	in.ContentHash = ""
	if in.Version == 0 {
		in.Version = CheckpointVersion
	}
	if err := validateCheckpoint(in); err != nil {
		return Checkpoint{}, err
	}
	in.ContentHash = checkpointHash(in)
	return in, nil
}

func SealResultReceipt(checkpoint Checkpoint, in ResultReceipt) (ResultReceipt, error) {
	if err := validateSealedCheckpoint(checkpoint); err != nil {
		return ResultReceipt{}, err
	}
	in.ReceiptHash = ""
	if in.Version == 0 {
		in.Version = ReceiptVersion
	}
	if err := validateReceiptBinding(checkpoint, in); err != nil {
		return ResultReceipt{}, err
	}
	in.ReceiptHash = receiptHash(in)
	return in, nil
}

func validateSealedCheckpoint(checkpoint Checkpoint) error {
	if err := validateCheckpoint(checkpoint); err != nil {
		return err
	}
	if checkpoint.ContentHash == "" || checkpoint.ContentHash != checkpointHash(checkpoint) {
		return fmt.Errorf("continuation checkpoint hash mismatch")
	}
	return nil
}

func validateCheckpoint(in Checkpoint) error {
	if in.Version != CheckpointVersion || in.ID == "" || in.InputID == "" {
		return fmt.Errorf("continuation requires supported version, ID, and input ID")
	}
	if err := in.Identity.Validate(); err != nil {
		return err
	}
	if in.Runtime.Harness == "" || in.Runtime.HarnessVersion == "" || in.Runtime.HarnessConfigDigest == "" ||
		!validResumeCapability(in.Runtime.ResumeCapability) ||
		in.Runtime.Model == "" || in.Runtime.EnvironmentDigest == "" || in.Runtime.PolicyVersion == "" || in.Runtime.CatalogVersion == "" {
		return fmt.Errorf("continuation requires a complete runtime binding")
	}
	if in.Context.ID == "" || in.Context.Revision == 0 || in.Context.Digest == "" || in.Context.Epoch == 0 {
		return fmt.Errorf("continuation requires a complete context binding")
	}
	if in.Tool.Name == "" || in.Tool.Version == "" || in.Tool.RoundID == "" || in.Tool.CallID == "" || in.Tool.AttemptID == "" ||
		in.Tool.ApprovalID == "" || in.Tool.EnvelopeHash == "" || in.Tool.TargetHash == "" || in.Tool.IdempotencyKey == "" {
		return fmt.Errorf("continuation requires an exact approved tool binding")
	}
	return nil
}

func validateReceiptBinding(checkpoint Checkpoint, receipt ResultReceipt) error {
	if receipt.Version != ReceiptVersion || receipt.ContinuationID != checkpoint.ID || receipt.CheckpointHash != checkpoint.ContentHash ||
		receipt.AttemptID != checkpoint.Tool.AttemptID || receipt.RoundID != checkpoint.Tool.RoundID ||
		receipt.CallID != checkpoint.Tool.CallID || receipt.ToolName != checkpoint.Tool.Name {
		return fmt.Errorf("tool result receipt does not match checkpoint")
	}
	if receipt.ResultEventID == "" || receipt.ResultEventHash == "" || receipt.SemanticResultHash == "" ||
		receipt.ObservableResultHash == "" || receipt.Projection.ID == "" || receipt.Projection.LogicalCallID == "" ||
		receipt.Projection.ToolName != checkpoint.Tool.Name {
		return fmt.Errorf("tool result receipt is incomplete")
	}
	switch receipt.EffectCertainty {
	case EffectNoEffect, EffectApplied, EffectUnknown:
	default:
		return fmt.Errorf("invalid effect certainty %q", receipt.EffectCertainty)
	}
	return nil
}

func checkpointHash(in Checkpoint) string {
	return hashSections(
		fmt.Sprint(in.Version), in.ID, in.Identity.PrincipalID, in.Identity.TenantID, in.Identity.SessionID,
		in.Identity.RunID, in.InputID, in.Runtime.Harness, in.Runtime.HarnessVersion,
		in.Runtime.HarnessConfigDigest, in.Runtime.ResumeCapability, in.Runtime.Model, in.Runtime.EnvironmentDigest,
		in.Runtime.PolicyVersion, in.Runtime.CatalogVersion, in.Context.ID, fmt.Sprint(in.Context.Revision),
		in.Context.Digest, fmt.Sprint(in.Context.Epoch), in.Tool.Name, in.Tool.Version, in.Tool.RoundID,
		in.Tool.CallID, in.Tool.AttemptID, in.Tool.ApprovalID, in.Tool.EnvelopeHash, in.Tool.TargetHash,
		in.Tool.IdempotencyKey, in.DelegationLeaseDigest, fmt.Sprint(in.TrajectoryCursor), in.ResumeTokenHash,
	)
}

func validResumeCapability(capability string) bool {
	switch capability {
	case "token", "transcript", "exact_checkpoint":
		return true
	default:
		return false
	}
}

func receiptHash(in ResultReceipt) string {
	return hashSections(
		fmt.Sprint(in.Version), in.ContinuationID, in.CheckpointHash, in.AttemptID, in.RoundID, in.CallID,
		in.ToolName, in.ResultEventID, in.ResultEventHash, in.SemanticResultHash, in.ObservableResultHash,
		fmt.Sprint(in.IsError), string(in.EffectCertainty), in.Projection.ID, in.Projection.LogicalCallID,
		in.Projection.ToolName,
	)
}

func hashSections(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		var size [8]byte
		n := uint64(len(part))
		for i := 7; i >= 0; i-- {
			size[i] = byte(n)
			n >>= 8
		}
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(part))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
