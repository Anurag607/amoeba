// Package conformance provides reusable behavioral probes for host-supplied
// durable stores. The probes intentionally avoid persistence technology.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Anurag607/amoeba/contextengine"
	"github.com/Anurag607/amoeba/continuation"
	"github.com/Anurag607/amoeba/execution"
	"github.com/Anurag607/amoeba/trajectory"
)

type CoreStores struct {
	Admissions execution.AdmissionStore
	Runs       execution.RunStore
	Attempts   execution.AttemptLedger
	Approvals  execution.ApprovalStore
	ToolRounds execution.ToolRoundStore
	Work       interface {
		execution.PlanStore
		execution.TaskStore
	}
	Contexts      contextengine.SnapshotStore
	Trajectory    trajectory.ReplayStore
	Continuations continuation.Store
}

// ProbeCore verifies owner isolation, exact retry behavior, CAS rejection,
// and monotonic cursors against a fresh store set.
func ProbeCore(ctx context.Context, stores CoreStores) error {
	if err := validateCoreStores(stores); err != nil {
		return err
	}
	owner := execution.Identity{PrincipalID: "conformance", SessionID: "session", RunID: "run"}
	other := execution.Identity{PrincipalID: "other", SessionID: "session", RunID: "run"}
	if err := probeAdmission(ctx, stores.Admissions, owner, other); err != nil {
		return fmt.Errorf("admission store: %w", err)
	}
	if err := probeRun(ctx, stores.Runs, owner, other); err != nil {
		return fmt.Errorf("run store: %w", err)
	}
	if err := probeAttempt(ctx, stores.Attempts, owner, other); err != nil {
		return fmt.Errorf("attempt ledger: %w", err)
	}
	if err := probeApproval(ctx, stores.Approvals, owner, other); err != nil {
		return fmt.Errorf("approval store: %w", err)
	}
	if err := probeToolRound(ctx, stores.ToolRounds, owner); err != nil {
		return fmt.Errorf("tool round store: %w", err)
	}
	if err := probeWork(ctx, stores.Work, owner); err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	if err := probeContext(ctx, stores.Contexts, owner, other); err != nil {
		return fmt.Errorf("context store: %w", err)
	}
	if err := probeTrajectory(ctx, stores.Trajectory, owner); err != nil {
		return fmt.Errorf("trajectory store: %w", err)
	}
	if err := probeContinuation(ctx, stores.Continuations, owner, other); err != nil {
		return fmt.Errorf("continuation store: %w", err)
	}
	return nil
}

func validateCoreStores(stores CoreStores) error {
	if stores.Admissions == nil || stores.Runs == nil || stores.Attempts == nil || stores.Approvals == nil || stores.ToolRounds == nil || stores.Work == nil || stores.Contexts == nil || stores.Trajectory == nil || stores.Continuations == nil {
		return fmt.Errorf("conformance probe requires every core store")
	}
	return nil
}

func probeContinuation(ctx context.Context, store continuation.Store, owner, other execution.Identity) error {
	checkpoint, err := continuation.SealCheckpoint(continuation.Checkpoint{
		ID: "conformance-continuation", Identity: owner, InputID: "conformance-input",
		Runtime: continuation.RuntimeBinding{Harness: "harness", HarnessVersion: "1", HarnessConfigDigest: "harness-digest", ResumeCapability: "exact_checkpoint", Model: "model", EnvironmentDigest: "environment-digest", PolicyVersion: "p", CatalogVersion: "c"},
		Context: continuation.ContextBinding{ID: "conformance-context", Revision: 1, Digest: "context-before", Epoch: 1},
		Tool:    continuation.ToolBinding{Name: "tool", Version: "1", RoundID: "round", CallID: "call", AttemptID: "attempt", ApprovalID: "approval", EnvelopeHash: "envelope", TargetHash: "target", IdempotencyKey: "key"},
	})
	if err != nil {
		return err
	}
	record, fresh, err := store.Create(ctx, checkpoint, time.Now())
	if err != nil || !fresh {
		return fmt.Errorf("create: fresh=%v err=%w", fresh, err)
	}
	if _, fresh, err := store.Create(ctx, checkpoint, record.CreatedAt); err != nil || fresh {
		return fmt.Errorf("exact retry: fresh=%v err=%w", fresh, err)
	}
	receipt, err := continuation.SealResultReceipt(checkpoint, continuation.ResultReceipt{
		ContinuationID: checkpoint.ID, CheckpointHash: checkpoint.ContentHash, AttemptID: checkpoint.Tool.AttemptID,
		RoundID: checkpoint.Tool.RoundID, CallID: checkpoint.Tool.CallID, ToolName: checkpoint.Tool.Name,
		ResultEventID: "result-event", ResultEventHash: "result-event-hash", SemanticResultHash: "semantic-hash",
		ObservableResultHash: "observable-hash", EffectCertainty: continuation.EffectNoEffect,
		Projection: continuation.ProjectionBinding{ID: "projection", LogicalCallID: "logical-call", ToolName: checkpoint.Tool.Name},
	})
	if err != nil {
		return err
	}
	if _, err := store.CommitResult(ctx, owner, checkpoint.ID, record.Revision+1, receipt, "outbox-result", time.Now()); !errors.Is(err, continuation.ErrRevisionConflict) {
		return fmt.Errorf("stale CAS was not rejected: %v", err)
	}
	committed, err := store.CommitResult(ctx, owner, checkpoint.ID, record.Revision, receipt, "outbox-result", time.Now())
	if err != nil {
		return fmt.Errorf("commit result: %w", err)
	}
	leased, err := store.LeaseOutbox(ctx, owner, "worker-a", "lease-a", 1, time.Now(), time.Minute)
	if err != nil || len(leased) != 1 || leased[0].ExpectedState != continuation.StateResultCommitted || leased[0].ExpectedRevision != committed.Revision || leased[0].Attempts != 1 || leased[0].ContentHash == "" {
		return fmt.Errorf("lease outbox: events=%+v err=%v", leased, err)
	}
	if _, err := store.AckLeasedOutbox(ctx, owner, leased[0].ID, "wrong-lease", leased[0].Attempts, time.Now()); !errors.Is(err, continuation.ErrOutboxLeaseConflict) {
		return fmt.Errorf("stale outbox lease was not rejected: %v", err)
	}
	retryAt := time.Now()
	if _, err := store.RetryOutbox(ctx, owner, leased[0].ID, leased[0].Lease.ID, leased[0].Attempts, "transient", retryAt, retryAt); err != nil {
		return fmt.Errorf("retry outbox: %w", err)
	}
	if _, found, err := store.Get(ctx, other, checkpoint.ID); err == nil && found {
		return fmt.Errorf("owner isolation was not enforced")
	}
	return nil
}

func probeAdmission(ctx context.Context, store execution.AdmissionStore, owner, other execution.Identity) error {
	input := execution.AdmittedInput{ID: "conformance-input", Identity: owner, PayloadHash: "sha256:input", Mode: execution.InputQueue}
	created, fresh, err := store.Create(ctx, input)
	if err != nil || !fresh {
		return fmt.Errorf("create: fresh=%v err=%w", fresh, err)
	}
	if _, fresh, err := store.Create(ctx, input); err != nil || fresh {
		return fmt.Errorf("exact retry: fresh=%v err=%w", fresh, err)
	}
	if _, err := store.Transition(ctx, owner, created.ID, created.Revision+1, execution.InputRunning, "", ""); !errors.Is(err, execution.ErrInputRevision) {
		return fmt.Errorf("stale CAS was not rejected: %v", err)
	}
	if _, _, err := store.Get(ctx, other, created.ID); err == nil {
		return fmt.Errorf("owner isolation was not enforced")
	}
	return nil
}

func probeRun(ctx context.Context, store execution.RunStore, owner, other execution.Identity) error {
	run, err := store.Create(ctx, execution.Run{ID: owner.RunID, InputID: "conformance-input", Identity: owner})
	if err != nil {
		return err
	}
	if _, err := store.Update(ctx, owner, run.ID, run.Revision+1, execution.RunExecuting, execution.DeliveryPending, "", 0); err == nil {
		return fmt.Errorf("stale CAS was not rejected")
	}
	if _, _, err := store.Get(ctx, other, run.ID); err == nil {
		return fmt.Errorf("owner isolation was not enforced")
	}
	return nil
}

func probeAttempt(ctx context.Context, store execution.AttemptLedger, owner, other execution.Identity) error {
	attempt := execution.Attempt{ID: "conformance-attempt", Identity: owner, Tool: execution.ToolRef{Name: "tool", Version: "1"}, PolicyVersion: "p", CatalogVersion: "c", SchemaDigest: "schema", ArgsHash: "args", TargetHash: "target", IdempotencyKey: "key"}
	created, fresh, err := store.Admit(ctx, attempt)
	if err != nil || !fresh {
		return fmt.Errorf("admit: fresh=%v err=%w", fresh, err)
	}
	if _, fresh, err := store.Admit(ctx, attempt); err != nil || fresh {
		return fmt.Errorf("exact retry: fresh=%v err=%w", fresh, err)
	}
	if _, err := store.Start(ctx, owner, created.ID, created.Revision+1, time.Now()); !errors.Is(err, execution.ErrAttemptRevisionConflict) {
		return fmt.Errorf("stale CAS was not rejected: %v", err)
	}
	if _, _, err := store.Get(ctx, other, created.ID); !errors.Is(err, execution.ErrAttemptOwnerMismatch) {
		return fmt.Errorf("owner isolation was not enforced: %v", err)
	}
	return nil
}

func probeApproval(ctx context.Context, store execution.ApprovalStore, owner, other execution.Identity) error {
	envelope := execution.OperationEnvelope{Version: execution.OperationEnvelopeVersion, Identity: owner, AttemptID: "approval-attempt", RequestID: "request", PolicyVersion: "p", IdempotencyKey: "key", Operation: execution.Operation{Tool: execution.ToolRef{Name: "tool", Version: "1"}, CatalogVersion: "c", SchemaDigest: "schema", CanonicalArguments: `{}`, ArgsHash: "args", TargetHash: "target"}}
	request, err := execution.NewApprovalRequest("approval", envelope, "conformance", time.Now())
	if err != nil {
		return err
	}
	record, fresh, err := store.Create(ctx, request)
	if err != nil || !fresh {
		return fmt.Errorf("create: fresh=%v err=%w", fresh, err)
	}
	decision := execution.ApprovalDecision{RequestID: request.ID, EnvelopeHash: request.EnvelopeHash, Allowed: true}
	if _, err := store.Resolve(ctx, owner, request.ID, record.Revision+1, decision); !errors.Is(err, execution.ErrApprovalRevisionConflict) {
		return fmt.Errorf("stale CAS was not rejected: %v", err)
	}
	if _, _, err := store.Get(ctx, other, request.ID); !errors.Is(err, execution.ErrApprovalOwnerMismatch) {
		return fmt.Errorf("owner isolation was not enforced: %v", err)
	}
	return nil
}

func probeToolRound(ctx context.Context, store execution.ToolRoundStore, owner execution.Identity) error {
	round := execution.ToolRound{ID: "conformance-round", Identity: owner, PolicyVersion: "p", CatalogVersion: "c", Calls: []execution.ToolCallRecord{{ID: "call", Tool: execution.ToolRef{Name: "tool", Version: "1"}, Required: true}}}
	created, fresh, err := store.Create(ctx, round)
	if err != nil || !fresh {
		return fmt.Errorf("create: fresh=%v err=%w", fresh, err)
	}
	if _, err := store.Transition(ctx, owner, created.ID, created.Revision+1, "call", execution.ToolCallAdmitted, "attempt", nil); !errors.Is(err, execution.ErrToolRoundRevision) {
		return fmt.Errorf("stale CAS was not rejected: %v", err)
	}
	return nil
}

func probeWork(ctx context.Context, store interface {
	execution.PlanStore
	execution.TaskStore
}, owner execution.Identity) error {
	plan, err := store.CreatePlan(ctx, execution.PlanRecord{Owner: owner, Plan: execution.Plan{ID: "conformance-plan", Steps: []execution.PlanStep{{ID: "step", Description: "probe", Status: execution.PlanStepPending}}}})
	if err != nil {
		return err
	}
	if _, err := store.UpdatePlan(ctx, owner, plan.Plan.ID, plan.Plan.Revision+1, plan.Plan.Steps); !errors.Is(err, execution.ErrPlanRevision) {
		return fmt.Errorf("stale plan CAS was not rejected: %v", err)
	}
	task, err := store.CreateTask(ctx, execution.Task{ID: "conformance-task", Owner: owner})
	if err != nil {
		return err
	}
	if _, err := store.TransitionTask(ctx, owner, task.ID, task.Revision+1, execution.TaskClaimed, "", "worker"); !errors.Is(err, execution.ErrTaskRevision) {
		return fmt.Errorf("stale task CAS was not rejected: %v", err)
	}
	return nil
}

func probeContext(ctx context.Context, store contextengine.SnapshotStore, owner, other execution.Identity) error {
	record, err := store.Create(ctx, contextengine.SnapshotRecord{ID: "conformance-context", Identity: owner, Snapshot: contextengine.Snapshot{Epoch: 1, Sources: map[string]contextengine.SourceState{}}})
	if err != nil {
		return err
	}
	if _, err := store.CompareAndSwap(ctx, owner, record.ID, record.Revision+1, record.Snapshot); err == nil {
		return fmt.Errorf("stale CAS was not rejected")
	}
	if _, _, err := store.Get(ctx, other, record.ID); err == nil {
		return fmt.Errorf("owner isolation was not enforced")
	}
	return nil
}

func probeTrajectory(ctx context.Context, store trajectory.ReplayStore, owner execution.Identity) error {
	event := trajectory.Event{ID: "conformance-event", SessionID: owner.SessionID, RunID: owner.RunID, Type: "probe"}
	first, err := store.Append(ctx, event)
	if err != nil {
		return err
	}
	retry, err := store.Append(ctx, event)
	if err != nil || retry.Sequence != first.Sequence {
		return fmt.Errorf("exact retry changed sequence: first=%d retry=%d err=%w", first.Sequence, retry.Sequence, err)
	}
	state, err := store.Ack(ctx, owner.SessionID, owner.RunID, first.Sequence)
	if err != nil || state.AckedSequence != first.Sequence {
		return fmt.Errorf("ack: state=%+v err=%w", state, err)
	}
	if _, err := store.Ack(ctx, owner.SessionID, owner.RunID, first.Sequence-1); err == nil {
		return fmt.Errorf("non-monotonic ACK was accepted")
	}
	return nil
}
