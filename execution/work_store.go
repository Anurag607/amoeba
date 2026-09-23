package execution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	PlanStepPending   = "pending"
	PlanStepRunning   = "running"
	PlanStepCompleted = "completed"
	PlanStepBlocked   = "blocked"

	TaskPending   = "pending"
	TaskClaimed   = "claimed"
	TaskRunning   = "running"
	TaskCompleted = "completed"
	TaskFailed    = "failed"
	TaskCancelled = "cancelled"
)

var (
	ErrPlanRevision = errors.New("plan revision conflict")
	ErrTaskRevision = errors.New("task revision conflict")
	ErrTaskClaimed  = errors.New("task is already claimed")
)

type PlanRecord struct {
	Owner Identity `json:"owner"`
	Plan  Plan     `json:"plan"`
}

type PlanStore interface {
	CreatePlan(context.Context, PlanRecord) (PlanRecord, error)
	UpdatePlan(context.Context, Identity, string, uint64, []PlanStep) (PlanRecord, error)
	GetPlan(context.Context, Identity, string) (PlanRecord, bool, error)
}

type TaskStore interface {
	CreateTask(context.Context, Task) (Task, error)
	TransitionTask(context.Context, Identity, string, uint64, string, string, string) (Task, error)
	GetTask(context.Context, Identity, string) (Task, bool, error)
	ListTasks(context.Context, Identity, string) ([]Task, error)
}

type MemoryWorkStore struct {
	mu    sync.Mutex
	plans map[string]PlanRecord
	tasks map[string]Task
}

func NewMemoryWorkStore() *MemoryWorkStore {
	return &MemoryWorkStore{plans: make(map[string]PlanRecord), tasks: make(map[string]Task)}
}

func (s *MemoryWorkStore) CreatePlan(ctx context.Context, record PlanRecord) (PlanRecord, error) {
	if err := ctx.Err(); err != nil {
		return PlanRecord{}, err
	}
	if err := record.Owner.Validate(); err != nil {
		return PlanRecord{}, err
	}
	if err := validatePlan(record.Plan); err != nil {
		return PlanRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.plans[record.Plan.ID]; exists {
		return PlanRecord{}, fmt.Errorf("plan %q already exists", record.Plan.ID)
	}
	record.Plan.Revision, record.Plan.UpdatedAt = 1, time.Now()
	record.Plan.Steps = clonePlanSteps(record.Plan.Steps)
	s.plans[record.Plan.ID] = record
	return clonePlanRecord(record), nil
}

func (s *MemoryWorkStore) UpdatePlan(ctx context.Context, owner Identity, id string, expected uint64, steps []PlanStep) (PlanRecord, error) {
	if err := ctx.Err(); err != nil {
		return PlanRecord{}, err
	}
	if err := validatePlan(Plan{ID: id, Steps: steps}); err != nil {
		return PlanRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.plans[id]
	if !ok {
		return PlanRecord{}, fmt.Errorf("plan %q not found", id)
	}
	if record.Owner != owner {
		return PlanRecord{}, fmt.Errorf("plan owner mismatch")
	}
	if record.Plan.Revision != expected {
		return PlanRecord{}, ErrPlanRevision
	}
	record.Plan.Steps = clonePlanSteps(steps)
	record.Plan.Revision++
	record.Plan.UpdatedAt = time.Now()
	s.plans[id] = record
	return clonePlanRecord(record), nil
}

func (s *MemoryWorkStore) GetPlan(ctx context.Context, owner Identity, id string) (PlanRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return PlanRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.plans[id]
	if !ok {
		return PlanRecord{}, false, nil
	}
	if record.Owner != owner {
		return PlanRecord{}, false, fmt.Errorf("plan owner mismatch")
	}
	return clonePlanRecord(record), true, nil
}

func (s *MemoryWorkStore) CreateTask(ctx context.Context, task Task) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	if task.ID == "" || task.Owner.Validate() != nil {
		return Task{}, fmt.Errorf("task requires ID and owner")
	}
	if task.Status == "" {
		task.Status = TaskPending
	}
	if task.Status != TaskPending {
		return Task{}, fmt.Errorf("new task must be pending")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[task.ID]; exists {
		return Task{}, fmt.Errorf("task %q already exists", task.ID)
	}
	if task.ParentTaskID != "" {
		parent, ok := s.tasks[task.ParentTaskID]
		if !ok || parent.Owner != task.Owner {
			return Task{}, fmt.Errorf("task parent is missing or has another owner")
		}
	}
	if task.PlanID != "" {
		plan, ok := s.plans[task.PlanID]
		if !ok || plan.Owner != task.Owner || plan.Plan.Revision != task.PlanRevision {
			return Task{}, fmt.Errorf("task plan binding is missing, stale, or owned by another identity")
		}
	}
	task.Revision, task.UpdatedAt = 1, time.Now()
	s.tasks[task.ID] = task
	return task, nil
}

func (s *MemoryWorkStore) TransitionTask(ctx context.Context, owner Identity, id string, expected uint64, status, runID, assignee string) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return Task{}, fmt.Errorf("task %q not found", id)
	}
	if task.Owner != owner {
		return Task{}, fmt.Errorf("task owner mismatch")
	}
	if task.Revision != expected {
		return Task{}, ErrTaskRevision
	}
	if task.Assignee != "" && assignee != "" && task.Assignee != assignee {
		return Task{}, ErrTaskClaimed
	}
	if err := validateTaskTransition(task.Status, status); err != nil {
		return Task{}, err
	}
	if (status == TaskClaimed || status == TaskRunning) && assignee == "" {
		return Task{}, fmt.Errorf("claimed or running task requires assignee")
	}
	if status == TaskRunning && runID == "" {
		return Task{}, fmt.Errorf("running task requires run ID")
	}
	task.Status = status
	if runID != "" {
		task.RunID = runID
	}
	if assignee != "" {
		task.Assignee = assignee
	}
	task.Revision++
	task.UpdatedAt = time.Now()
	s.tasks[id] = task
	return task, nil
}

func (s *MemoryWorkStore) GetTask(ctx context.Context, owner Identity, id string) (Task, bool, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return Task{}, false, nil
	}
	if task.Owner != owner {
		return Task{}, false, fmt.Errorf("task owner mismatch")
	}
	return task, true, nil
}

func (s *MemoryWorkStore) ListTasks(ctx context.Context, owner Identity, goalID string) ([]Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, 0)
	for _, task := range s.tasks {
		if task.Owner == owner && (goalID == "" || task.GoalID == goalID) {
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func validatePlan(plan Plan) error {
	if plan.ID == "" || len(plan.Steps) == 0 {
		return fmt.Errorf("plan requires ID and steps")
	}
	seen := make(map[string]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.ID == "" || step.Description == "" {
			return fmt.Errorf("plan step requires ID and description")
		}
		if step.Status != PlanStepPending && step.Status != PlanStepRunning && step.Status != PlanStepCompleted && step.Status != PlanStepBlocked {
			return fmt.Errorf("plan step %q has invalid status %q", step.ID, step.Status)
		}
		if _, duplicate := seen[step.ID]; duplicate {
			return fmt.Errorf("duplicate plan step %q", step.ID)
		}
		seen[step.ID] = struct{}{}
	}
	return nil
}

func validateTaskTransition(from, to string) error {
	valid := false
	switch from {
	case TaskPending:
		valid = to == TaskClaimed || to == TaskCancelled
	case TaskClaimed:
		valid = to == TaskRunning || to == TaskPending || to == TaskCancelled
	case TaskRunning:
		valid = to == TaskCompleted || to == TaskFailed || to == TaskCancelled
	}
	if !valid {
		return fmt.Errorf("invalid task transition %s -> %s", from, to)
	}
	return nil
}

func clonePlanSteps(in []PlanStep) []PlanStep { return append([]PlanStep(nil), in...) }
func clonePlanRecord(in PlanRecord) PlanRecord {
	in.Plan.Steps = clonePlanSteps(in.Plan.Steps)
	return in
}
