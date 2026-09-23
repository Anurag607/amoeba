package execution

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type ChildStatus string

const (
	ChildPending   ChildStatus = "pending"
	ChildRunning   ChildStatus = "running"
	ChildCompleted ChildStatus = "completed"
	ChildFailed    ChildStatus = "failed"
	ChildCancelled ChildStatus = "cancelled"
	ChildPressure  ChildStatus = "pressure"
)

func (s ChildStatus) terminal() bool {
	return s == ChildCompleted || s == ChildFailed || s == ChildCancelled || s == ChildPressure
}

// ChildRun is the optional durable projection for inspectable or detached
// delegated work. It intentionally stores manifests and artifacts, not a full
// child transcript.
type ChildRun struct {
	ID                string      `json:"id"`
	ParentRunID       string      `json:"parent_run_id"`
	ParentNodeID      string      `json:"parent_node_id,omitempty"`
	ExpertID          string      `json:"expert_id"`
	Identity          Identity    `json:"identity"`
	Model             string      `json:"model"`
	CapabilityVersion string      `json:"capability_version"`
	ContextDigest     string      `json:"context_digest,omitempty"`
	Status            ChildStatus `json:"status"`
	ArtifactRef       string      `json:"artifact_ref,omitempty"`
	FailureCode       string      `json:"failure_code,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	Revision          uint64      `json:"revision"`
}

type ChildRunStore interface {
	Create(context.Context, ChildRun) error
	Transition(context.Context, string, uint64, ChildStatus, string, string) (ChildRun, error)
	Get(context.Context, string) (ChildRun, bool)
}

type MemoryChildRunStore struct {
	mu   sync.Mutex
	runs map[string]ChildRun
}

func NewMemoryChildRunStore() *MemoryChildRunStore {
	return &MemoryChildRunStore{runs: make(map[string]ChildRun)}
}

func (s *MemoryChildRunStore) Create(ctx context.Context, run ChildRun) error {
	if run.ID == "" || run.ParentRunID == "" || run.ExpertID == "" {
		return fmt.Errorf("child run requires id, parent run, and expert")
	}
	if err := run.Identity.Validate(); err != nil {
		return err
	}
	if err := requireContextOwner(ctx, run.Identity); err != nil {
		return fmt.Errorf("create child run: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return fmt.Errorf("child run %q already exists", run.ID)
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	run.UpdatedAt, run.Status, run.Revision = run.CreatedAt, ChildPending, 1
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryChildRunStore) Transition(ctx context.Context, id string, expected uint64, next ChildStatus, artifactRef, failureCode string) (ChildRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return ChildRun{}, fmt.Errorf("child run %q not found", id)
	}
	if err := requireContextOwner(ctx, run.Identity); err != nil {
		return ChildRun{}, err
	}
	if run.Revision != expected {
		return ChildRun{}, fmt.Errorf("child run %q revision conflict", id)
	}
	if run.Status.terminal() {
		return ChildRun{}, fmt.Errorf("child run %q is terminal", id)
	}
	if run.Status == ChildPending && next != ChildRunning && next != ChildCancelled && next != ChildPressure {
		return ChildRun{}, fmt.Errorf("invalid child transition %s -> %s", run.Status, next)
	}
	if run.Status == ChildRunning && next != ChildCompleted && next != ChildFailed && next != ChildCancelled && next != ChildPressure {
		return ChildRun{}, fmt.Errorf("invalid child transition %s -> %s", run.Status, next)
	}
	run.Status, run.ArtifactRef, run.FailureCode = next, artifactRef, failureCode
	run.Revision++
	run.UpdatedAt = time.Now()
	s.runs[id] = run
	return run, nil
}

func (s *MemoryChildRunStore) Get(ctx context.Context, id string) (ChildRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || requireContextOwner(ctx, run.Identity) != nil {
		return ChildRun{}, false
	}
	return run, true
}

func requireContextOwner(ctx context.Context, want Identity) error {
	owner, ok := IdentityFromContext(ctx)
	if !ok {
		return fmt.Errorf("operation has no admitted identity")
	}
	if owner != want {
		return fmt.Errorf("owner mismatch")
	}
	return nil
}
