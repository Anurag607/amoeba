package execution

import (
	"context"
	"errors"
	"testing"
)

func TestWorkStoreRevisionsPlansAndClaimsTasks(t *testing.T) {
	ctx := context.Background()
	owner := Identity{PrincipalID: "u", SessionID: "s", RunID: "root"}
	store := NewMemoryWorkStore()
	plan, err := store.CreatePlan(ctx, PlanRecord{Owner: owner, Plan: Plan{ID: "p", Steps: []PlanStep{{ID: "s1", Description: "inspect", Status: PlanStepPending}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePlan(ctx, owner, "p", plan.Plan.Revision+1, plan.Plan.Steps); !errors.Is(err, ErrPlanRevision) {
		t.Fatalf("stale plan update=%v", err)
	}
	parent, err := store.CreateTask(ctx, Task{ID: "t1", GoalID: "g", PlanID: plan.Plan.ID, PlanRevision: plan.Plan.Revision, Owner: owner})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateTask(ctx, Task{ID: "t2", GoalID: "g", ParentTaskID: parent.ID, Owner: owner})
	if err != nil {
		t.Fatal(err)
	}
	child, err = store.TransitionTask(ctx, owner, child.ID, child.Revision, TaskClaimed, "", "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionTask(ctx, owner, child.ID, child.Revision, TaskRunning, "run-2", "worker-b"); !errors.Is(err, ErrTaskClaimed) {
		t.Fatalf("task reassignment=%v", err)
	}
	updated, err := store.UpdatePlan(ctx, owner, plan.Plan.ID, plan.Plan.Revision, []PlanStep{{ID: "s1", Description: "inspect", Status: PlanStepCompleted}})
	if err != nil || !IsTaskPlanStale(parent, updated.Plan) {
		t.Fatalf("updated=%+v stale=%v err=%v", updated, IsTaskPlanStale(parent, updated.Plan), err)
	}
}
