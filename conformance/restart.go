package conformance

import (
	"context"
	"fmt"
	"time"

	"github.com/anurgosw/agentic-moe/contextengine"
	"github.com/anurgosw/agentic-moe/continuation"
	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/trajectory"
)

// RestartHarness opens the same durable backing store on every call. Close
// must release all process-local handles before the next Open.
type RestartHarness struct {
	Open func(context.Context) (CoreStores, func() error, error)
}

// ProbeRestart writes nonterminal and reconciliation-sensitive state, closes
// the implementation, reopens it, and verifies that recovery facts survived.
func ProbeRestart(ctx context.Context, harness RestartHarness) (retErr error) {
	if harness.Open == nil {
		return fmt.Errorf("restart conformance requires an open function")
	}
	stores, closeStore, err := harness.Open(ctx)
	if err != nil {
		return err
	}
	if closeStore == nil {
		return fmt.Errorf("restart conformance requires a close function")
	}
	if err := validateCoreStores(stores); err != nil {
		_ = closeStore()
		return err
	}
	owner := execution.Identity{PrincipalID: "restart", SessionID: "restart-session", RunID: "restart-run"}
	input, _, err := stores.Admissions.Create(ctx, execution.AdmittedInput{ID: "restart-input", Identity: owner, PayloadHash: "sha256:restart", Mode: execution.InputQueue})
	if err == nil {
		input, err = stores.Admissions.Transition(ctx, owner, input.ID, input.Revision, execution.InputRunning, "", "")
	}
	run := execution.Run{}
	if err == nil {
		run, err = stores.Runs.Create(ctx, execution.Run{ID: owner.RunID, InputID: input.ID, Identity: owner})
	}
	if err == nil {
		run, err = stores.Runs.Update(ctx, owner, run.ID, run.Revision, execution.RunPaused, execution.DeliveryPending, "", 0)
	}
	attempt := execution.Attempt{}
	if err == nil {
		attempt, _, err = stores.Attempts.Admit(ctx, execution.Attempt{ID: "restart-attempt", Identity: owner, Tool: execution.ToolRef{Name: "write", Version: "1"}, PolicyVersion: "p", CatalogVersion: "c", SchemaDigest: "schema", ArgsHash: "args", TargetHash: "target", IdempotencyKey: "restart"})
	}
	if err == nil {
		attempt, err = stores.Attempts.Start(ctx, owner, attempt.ID, attempt.Revision, time.Now())
	}
	if err == nil {
		attempt, err = stores.Attempts.Settle(ctx, owner, attempt.ID, attempt.Revision, execution.AttemptSettlement{State: execution.AttemptIndeterminate, ReasonCode: "restart_probe", ReconcileAction: "inspect", At: time.Now()})
	}
	if err == nil {
		envelope := execution.OperationEnvelope{Version: execution.OperationEnvelopeVersion, Identity: owner, AttemptID: "restart-approval-attempt", PolicyVersion: "p", IdempotencyKey: "approval", Operation: execution.Operation{Tool: execution.ToolRef{Name: "write", Version: "1"}, CatalogVersion: "c", SchemaDigest: "schema", CanonicalArguments: `{}`, ArgsHash: "args", TargetHash: "target"}}
		request, requestErr := execution.NewApprovalRequest("restart-approval", envelope, "restart_probe", time.Now())
		if requestErr != nil {
			err = requestErr
		} else {
			_, _, err = stores.Approvals.Create(ctx, request)
		}
	}
	if err == nil {
		_, _, err = stores.ToolRounds.Create(ctx, execution.ToolRound{ID: "restart-round", Identity: owner, PolicyVersion: "p", CatalogVersion: "c", Calls: []execution.ToolCallRecord{{ID: "call", Tool: execution.ToolRef{Name: "write", Version: "1"}, Required: true}}})
	}
	if err == nil {
		_, err = stores.Work.CreatePlan(ctx, execution.PlanRecord{Owner: owner, Plan: execution.Plan{ID: "restart-plan", Steps: []execution.PlanStep{{ID: "step", Description: "recover", Status: execution.PlanStepPending}}}})
	}
	if err == nil {
		_, err = stores.Work.CreateTask(ctx, execution.Task{ID: "restart-task", Owner: owner})
	}
	if err == nil {
		_, err = stores.Contexts.Create(ctx, contextengine.SnapshotRecord{ID: "restart-context", Identity: owner, Snapshot: contextengine.Snapshot{Epoch: 1, Sources: map[string]contextengine.SourceState{}}})
	}
	if err == nil {
		_, err = stores.Trajectory.Append(ctx, trajectory.Event{ID: "restart-event", SessionID: owner.SessionID, RunID: owner.RunID, Type: "checkpoint"})
	}
	continuationRecord := continuation.Record{}
	outboxLeaseAt := time.Time{}
	if err == nil {
		checkpoint, checkpointErr := continuation.SealCheckpoint(continuation.Checkpoint{
			ID: "restart-continuation", Identity: owner, InputID: input.ID,
			Runtime:          continuation.RuntimeBinding{Harness: "harness", HarnessVersion: "1", HarnessConfigDigest: "harness-digest", ResumeCapability: "exact_checkpoint", Model: "model", EnvironmentDigest: "environment-digest", PolicyVersion: "p", CatalogVersion: "c"},
			Context:          continuation.ContextBinding{ID: "restart-context", Revision: 1, Digest: "context-before", Epoch: 1},
			Tool:             continuation.ToolBinding{Name: "write", Version: "1", RoundID: "restart-round", CallID: "call", AttemptID: "restart-attempt", ApprovalID: "restart-approval", EnvelopeHash: "envelope", TargetHash: "target", IdempotencyKey: "restart"},
			TrajectoryCursor: 1,
		})
		if checkpointErr != nil {
			err = checkpointErr
		} else {
			continuationRecord, _, err = stores.Continuations.Create(ctx, checkpoint, time.Now())
			if err == nil {
				receipt, receiptErr := continuation.SealResultReceipt(checkpoint, continuation.ResultReceipt{
					ContinuationID: checkpoint.ID, CheckpointHash: checkpoint.ContentHash, AttemptID: checkpoint.Tool.AttemptID,
					RoundID: checkpoint.Tool.RoundID, CallID: checkpoint.Tool.CallID, ToolName: checkpoint.Tool.Name,
					ResultEventID: "restart-result-event", ResultEventHash: "result-event-hash", SemanticResultHash: "semantic-hash",
					ObservableResultHash: "observable-hash", EffectCertainty: continuation.EffectNoEffect,
					Projection: continuation.ProjectionBinding{ID: "restart-projection", LogicalCallID: "restart-call", ToolName: checkpoint.Tool.Name},
				})
				if receiptErr != nil {
					err = receiptErr
				} else {
					continuationRecord, err = stores.Continuations.CommitResult(ctx, owner, checkpoint.ID, continuationRecord.Revision, receipt, "restart-continuation-event", time.Now())
				}
			}
		}
	}
	if err == nil {
		outboxLeaseAt = time.Now()
		leased, leaseErr := stores.Continuations.LeaseOutbox(ctx, owner, "worker-before-restart", "lease-before-restart", 1, outboxLeaseAt, time.Millisecond)
		if leaseErr != nil {
			err = leaseErr
		} else if len(leased) != 1 || leased[0].Attempts != 1 {
			err = fmt.Errorf("lease continuation outbox before restart: %+v", leased)
		}
	}
	if closeErr := closeStore(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("seed restart state: %w", err)
	}

	reopened, closeReopened, err := harness.Open(ctx)
	if err != nil {
		return err
	}
	if closeReopened == nil {
		return fmt.Errorf("restart conformance requires a close function after reopen")
	}
	defer func() {
		if closeErr := closeReopened(); retErr == nil && closeErr != nil {
			retErr = fmt.Errorf("close reopened conformance store: %w", closeErr)
		}
	}()
	if err := validateCoreStores(reopened); err != nil {
		return err
	}
	if pending, err := reopened.Admissions.Pending(ctx, owner, 10); err != nil || len(pending) != 1 || pending[0].State != execution.InputRunning {
		return fmt.Errorf("admission did not survive restart: pending=%+v err=%v", pending, err)
	}
	if restored, ok, err := reopened.Runs.Get(ctx, owner, run.ID); err != nil || !ok || restored.Execution != execution.RunPaused {
		return fmt.Errorf("run did not survive restart: run=%+v ok=%v err=%v", restored, ok, err)
	}
	if unresolved, err := reopened.Attempts.ListForReconciliation(ctx, owner, 10); err != nil || len(unresolved) != 1 || unresolved[0].ID != attempt.ID {
		return fmt.Errorf("reconciliation attempt did not survive restart: attempts=%+v err=%v", unresolved, err)
	}
	if approval, ok, err := reopened.Approvals.Get(ctx, owner, "restart-approval"); err != nil || !ok || approval.State != execution.ApprovalPending {
		return fmt.Errorf("approval did not survive restart: approval=%+v ok=%v err=%v", approval, ok, err)
	}
	if rounds, err := reopened.ToolRounds.Open(ctx, owner, 10); err != nil || len(rounds) != 1 {
		return fmt.Errorf("tool round did not survive restart: rounds=%+v err=%v", rounds, err)
	}
	if _, ok, err := reopened.Work.GetPlan(ctx, owner, "restart-plan"); err != nil || !ok {
		return fmt.Errorf("plan did not survive restart: ok=%v err=%v", ok, err)
	}
	if _, ok, err := reopened.Work.GetTask(ctx, owner, "restart-task"); err != nil || !ok {
		return fmt.Errorf("task did not survive restart: ok=%v err=%v", ok, err)
	}
	if _, ok, err := reopened.Contexts.Get(ctx, owner, "restart-context"); err != nil || !ok {
		return fmt.Errorf("context did not survive restart: ok=%v err=%v", ok, err)
	}
	if events, err := reopened.Trajectory.Replay(ctx, owner.SessionID, owner.RunID, 0, 10); err != nil || len(events) != 1 {
		return fmt.Errorf("trajectory did not survive restart: events=%+v err=%v", events, err)
	}
	if restored, ok, err := reopened.Continuations.Get(ctx, owner, "restart-continuation"); err != nil || !ok || restored.State != continuation.StateResultCommitted || restored.Revision != continuationRecord.Revision {
		return fmt.Errorf("continuation did not survive restart: continuation=%+v ok=%v err=%v", restored, ok, err)
	}
	events, err := reopened.Continuations.LeaseOutbox(ctx, owner, "worker-after-restart", "lease-after-restart", 1, outboxLeaseAt.Add(time.Second), time.Minute)
	if err != nil || len(events) != 1 || events[0].ID != "restart-continuation-event" || events[0].Attempts != 2 {
		return fmt.Errorf("continuation outbox lease did not recover after restart: events=%+v err=%v", events, err)
	}
	if _, err := reopened.Continuations.AckLeasedOutbox(ctx, owner, events[0].ID, events[0].Lease.ID, events[0].Attempts, outboxLeaseAt.Add(2*time.Second)); err != nil {
		return fmt.Errorf("ack recovered continuation outbox: %w", err)
	}
	if pending, err := reopened.Continuations.PendingOutbox(ctx, owner, 10); err != nil || len(pending) != 0 {
		return fmt.Errorf("delivered continuation tombstone replayed: events=%+v err=%v", pending, err)
	}
	return nil
}
