package execution

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type RunExecutionState string
type RunDeliveryState string

const (
	RunPending   RunExecutionState = "pending"
	RunExecuting RunExecutionState = "executing"
	RunPaused    RunExecutionState = "paused"
	RunSucceeded RunExecutionState = "succeeded"
	RunFailed    RunExecutionState = "failed"
	RunCancelled RunExecutionState = "cancelled"

	DeliveryPending   RunDeliveryState = "pending"
	DeliveryStreaming RunDeliveryState = "streaming"
	DeliveryDelivered RunDeliveryState = "delivered"
	DeliveryFailed    RunDeliveryState = "failed"
)

type Run struct {
	ID             string            `json:"id"`
	InputID        string            `json:"input_id"`
	Identity       Identity          `json:"identity"`
	Execution      RunExecutionState `json:"execution"`
	Delivery       RunDeliveryState  `json:"delivery"`
	FailureCode    string            `json:"failure_code,omitempty"`
	DeliveryCursor uint64            `json:"delivery_cursor,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Revision       uint64            `json:"revision"`
}

type RunStore interface {
	Create(context.Context, Run) (Run, error)
	Update(context.Context, Identity, string, uint64, RunExecutionState, RunDeliveryState, string, uint64) (Run, error)
	Get(context.Context, Identity, string) (Run, bool, error)
}

type MemoryRunStore struct {
	mu   sync.Mutex
	runs map[string]Run
}

func NewMemoryRunStore() *MemoryRunStore { return &MemoryRunStore{runs: make(map[string]Run)} }

func (s *MemoryRunStore) Create(ctx context.Context, run Run) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if run.ID == "" || run.InputID == "" {
		return Run{}, fmt.Errorf("run requires run and input IDs")
	}
	if err := run.Identity.Validate(); err != nil {
		return Run{}, err
	}
	if run.Identity.RunID != run.ID {
		return Run{}, fmt.Errorf("run identity does not match run ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.ID]; ok {
		return Run{}, fmt.Errorf("run %q already exists", run.ID)
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	run.UpdatedAt, run.Execution, run.Delivery, run.Revision = run.CreatedAt, RunPending, DeliveryPending, 1
	s.runs[run.ID] = run
	return run, nil
}

func (s *MemoryRunStore) Update(ctx context.Context, owner Identity, id string, expected uint64, execution RunExecutionState, delivery RunDeliveryState, failure string, cursor uint64) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, fmt.Errorf("run %q not found", id)
	}
	if run.Identity != owner {
		return Run{}, fmt.Errorf("run owner mismatch")
	}
	if run.Revision != expected {
		return Run{}, fmt.Errorf("run revision conflict")
	}
	if !validRunStates(execution, delivery) || cursor < run.DeliveryCursor {
		return Run{}, fmt.Errorf("invalid run projection update")
	}
	run.Execution, run.Delivery, run.FailureCode, run.DeliveryCursor = execution, delivery, failure, cursor
	run.Revision++
	run.UpdatedAt = time.Now()
	s.runs[id] = run
	return run, nil
}

func (s *MemoryRunStore) Get(ctx context.Context, owner Identity, id string) (Run, bool, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, false, nil
	}
	if run.Identity != owner {
		return Run{}, false, fmt.Errorf("run owner mismatch")
	}
	return run, true, nil
}

func validRunStates(execution RunExecutionState, delivery RunDeliveryState) bool {
	switch execution {
	case RunPending, RunExecuting, RunPaused, RunSucceeded, RunFailed, RunCancelled:
	default:
		return false
	}
	switch delivery {
	case DeliveryPending, DeliveryStreaming, DeliveryDelivered, DeliveryFailed:
		return true
	default:
		return false
	}
}
