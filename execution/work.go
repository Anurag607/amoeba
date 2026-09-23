package execution

import "time"

// Plan describes intended steps and may be revised without becoming a task.
type Plan struct {
	ID        string     `json:"id"`
	Revision  uint64     `json:"revision"`
	Steps     []PlanStep `json:"steps"`
	UpdatedAt time.Time  `json:"updated_at"`
}
type PlanStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// Goal is a durable user objective independent of any one run.
type Goal struct {
	ID        string `json:"id"`
	Objective string `json:"objective"`
	Status    string `json:"status"`
	Revision  uint64 `json:"revision"`
}

// Task is one executing unit with ownership and terminal state.
type Task struct {
	ID           string    `json:"id"`
	GoalID       string    `json:"goal_id,omitempty"`
	ParentTaskID string    `json:"parent_task_id,omitempty"`
	PlanID       string    `json:"plan_id,omitempty"`
	PlanRevision uint64    `json:"plan_revision,omitempty"`
	RunID        string    `json:"run_id,omitempty"`
	Status       string    `json:"status"`
	Assignee     string    `json:"assignee,omitempty"`
	Owner        Identity  `json:"owner"`
	Revision     uint64    `json:"revision"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IsTaskPlanStale reports whether a task was admitted against an older plan
// revision. Hosts can surface this without mutating either durable record.
func IsTaskPlanStale(task Task, plan Plan) bool {
	return task.PlanID != "" && task.PlanID == plan.ID && task.PlanRevision != plan.Revision
}

// Flow describes dependencies between tasks. It is a contract only; the
// framework intentionally does not provide a workflow scheduler.
type Flow struct {
	ID       string     `json:"id"`
	Revision uint64     `json:"revision"`
	Nodes    []FlowNode `json:"nodes"`
}
type FlowNode struct {
	TaskID    string   `json:"task_id"`
	DependsOn []string `json:"depends_on,omitempty"`
}
