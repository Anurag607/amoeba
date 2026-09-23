package execution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// AttemptState is the durable lifecycle of one invocation.
type AttemptState string

const (
	AttemptAdmitted        AttemptState = "admitted"
	AttemptExecuting       AttemptState = "executing"
	AttemptSucceeded       AttemptState = "succeeded"
	AttemptFailed          AttemptState = "failed"
	AttemptDenied          AttemptState = "denied"
	AttemptIndeterminate   AttemptState = "indeterminate"
	AttemptCancelledBefore AttemptState = "cancelled_before_effect"
)

func (s AttemptState) terminal() bool {
	switch s {
	case AttemptSucceeded, AttemptFailed, AttemptDenied, AttemptIndeterminate, AttemptCancelledBefore:
		return true
	default:
		return false
	}
}

// Attempt records immutable intent plus CAS-guarded lifecycle settlement.
type Attempt struct {
	ID              string       `json:"id"`
	Identity        Identity     `json:"identity"`
	RequestID       string       `json:"request_id,omitempty"`
	Tool            ToolRef      `json:"tool"`
	PolicyVersion   string       `json:"policy_version"`
	CatalogVersion  string       `json:"catalog_version"`
	SchemaDigest    string       `json:"schema_digest"`
	ArgsHash        string       `json:"args_hash"`
	TargetHash      string       `json:"target_hash"`
	IdempotencyKey  string       `json:"idempotency_key"`
	SourceEventIDs  []string     `json:"source_event_ids,omitempty"`
	State           AttemptState `json:"state"`
	ReasonCode      string       `json:"reason_code,omitempty"`
	ResultHash      string       `json:"result_hash,omitempty"`
	ReconcileAction string       `json:"reconcile_action,omitempty"`
	AdmittedAt      time.Time    `json:"admitted_at"`
	StartedAt       time.Time    `json:"started_at,omitempty"`
	SettledAt       time.Time    `json:"settled_at,omitempty"`
	Revision        uint64       `json:"revision"`
}

type AttemptSettlement struct {
	State           AttemptState
	ReasonCode      string
	ResultHash      string
	ReconcileAction string
	At              time.Time
}

var (
	ErrAttemptConflict         = errors.New("attempt identity reused with different input")
	ErrAttemptRevisionConflict = errors.New("attempt revision conflict")
	ErrAttemptOwnerMismatch    = errors.New("attempt owner mismatch")
	ErrAttemptTerminal         = errors.New("attempt already settled")
)

// AttemptLedger is the durable, owner-scoped CAS boundary used by Dispatcher.
type AttemptLedger interface {
	Admit(context.Context, Attempt) (Attempt, bool, error)
	Start(context.Context, Identity, string, uint64, time.Time) (Attempt, error)
	Settle(context.Context, Identity, string, uint64, AttemptSettlement) (Attempt, error)
	Get(context.Context, Identity, string) (Attempt, bool, error)
	ListForReconciliation(context.Context, Identity, int) ([]Attempt, error)
}

// MemoryAttemptLedger is a concurrency-safe reference implementation.
type MemoryAttemptLedger struct {
	mu       sync.Mutex
	attempts map[string]Attempt
}

func NewMemoryAttemptLedger() *MemoryAttemptLedger {
	return &MemoryAttemptLedger{attempts: make(map[string]Attempt)}
}

func (l *MemoryAttemptLedger) Admit(ctx context.Context, in Attempt) (Attempt, bool, error) {
	if err := ctx.Err(); err != nil {
		return Attempt{}, false, err
	}
	if in.ID == "" {
		return Attempt{}, false, fmt.Errorf("attempt ID is required")
	}
	if err := validateAttemptIntent(in); err != nil {
		return Attempt{}, false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.attempts[in.ID]; ok {
		if !sameAttemptIntent(existing, in) {
			return Attempt{}, false, ErrAttemptConflict
		}
		return cloneAttempt(existing), false, nil
	}
	in.State, in.Revision = AttemptAdmitted, 1
	in.SourceEventIDs = append([]string(nil), in.SourceEventIDs...)
	l.attempts[in.ID] = in
	return cloneAttempt(in), true, nil
}

func (l *MemoryAttemptLedger) Start(ctx context.Context, owner Identity, id string, expected uint64, at time.Time) (Attempt, error) {
	if err := ctx.Err(); err != nil {
		return Attempt{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, err := l.owned(id, owner)
	if err != nil {
		return Attempt{}, err
	}
	if attempt.Revision != expected {
		return Attempt{}, ErrAttemptRevisionConflict
	}
	if attempt.State != AttemptAdmitted {
		return Attempt{}, fmt.Errorf("start attempt %q from %s", id, attempt.State)
	}
	attempt.State, attempt.StartedAt = AttemptExecuting, at
	attempt.Revision++
	l.attempts[id] = attempt
	return cloneAttempt(attempt), nil
}

func (l *MemoryAttemptLedger) Settle(ctx context.Context, owner Identity, id string, expected uint64, settlement AttemptSettlement) (Attempt, error) {
	if err := ctx.Err(); err != nil {
		return Attempt{}, err
	}
	if !settlement.State.terminal() {
		return Attempt{}, fmt.Errorf("attempt settlement %q is not terminal", settlement.State)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, err := l.owned(id, owner)
	if err != nil {
		return Attempt{}, err
	}
	if attempt.Revision != expected {
		return Attempt{}, ErrAttemptRevisionConflict
	}
	if attempt.State.terminal() {
		return Attempt{}, ErrAttemptTerminal
	}
	if attempt.State != AttemptAdmitted && attempt.State != AttemptExecuting {
		return Attempt{}, fmt.Errorf("settle attempt %q from %s", id, attempt.State)
	}
	attempt.State = settlement.State
	attempt.ReasonCode = settlement.ReasonCode
	attempt.ResultHash = settlement.ResultHash
	attempt.ReconcileAction = settlement.ReconcileAction
	attempt.SettledAt = settlement.At
	attempt.Revision++
	l.attempts[id] = attempt
	return cloneAttempt(attempt), nil
}

func (l *MemoryAttemptLedger) Get(ctx context.Context, owner Identity, id string) (Attempt, bool, error) {
	if err := ctx.Err(); err != nil {
		return Attempt{}, false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, ok := l.attempts[id]
	if !ok {
		return Attempt{}, false, nil
	}
	if attempt.Identity != owner {
		return Attempt{}, false, ErrAttemptOwnerMismatch
	}
	return cloneAttempt(attempt), true, nil
}

func (l *MemoryAttemptLedger) ListForReconciliation(ctx context.Context, owner Identity, limit int) ([]Attempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("reconciliation limit must be positive")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Attempt, 0, limit)
	for _, attempt := range l.attempts {
		if attempt.Identity == owner && attempt.State == AttemptIndeterminate {
			out = append(out, cloneAttempt(attempt))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SettledAt.After(out[j].SettledAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (l *MemoryAttemptLedger) owned(id string, owner Identity) (Attempt, error) {
	attempt, ok := l.attempts[id]
	if !ok {
		return Attempt{}, fmt.Errorf("attempt %q not found", id)
	}
	if attempt.Identity != owner {
		return Attempt{}, ErrAttemptOwnerMismatch
	}
	return attempt, nil
}

func validateAttemptIntent(in Attempt) error {
	if err := in.Identity.Validate(); err != nil {
		return err
	}
	if in.Tool.Name == "" || in.Tool.Version == "" || in.ArgsHash == "" || in.TargetHash == "" {
		return fmt.Errorf("attempt requires a versioned tool, argument hash, and target hash")
	}
	if in.PolicyVersion == "" || in.CatalogVersion == "" || in.SchemaDigest == "" || in.IdempotencyKey == "" {
		return fmt.Errorf("attempt requires policy, catalog, schema, and idempotency bindings")
	}
	return nil
}

func sameAttemptIntent(a, b Attempt) bool {
	return a.Identity == b.Identity && a.RequestID == b.RequestID && a.Tool == b.Tool &&
		a.PolicyVersion == b.PolicyVersion && a.CatalogVersion == b.CatalogVersion &&
		a.SchemaDigest == b.SchemaDigest && a.ArgsHash == b.ArgsHash && a.TargetHash == b.TargetHash &&
		a.IdempotencyKey == b.IdempotencyKey && equalStrings(a.SourceEventIDs, b.SourceEventIDs)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneAttempt(in Attempt) Attempt {
	in.SourceEventIDs = append([]string(nil), in.SourceEventIDs...)
	return in
}
