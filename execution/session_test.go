package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSessionCoordinatorSerializesQueuesSteersAndJoins(t *testing.T) {
	store := NewMemoryAdmissionStore()
	coordinator, err := NewSessionCoordinator(store, 1)
	if err != nil {
		t.Fatal(err)
	}
	owner := Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	first, err := coordinator.Admit(context.Background(), AdmittedInput{ID: "i1", Identity: owner, PayloadHash: "h1", Mode: InputQueue})
	if err != nil || first.Input.State != InputRunning || !first.Wake {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	join, err := coordinator.Admit(context.Background(), AdmittedInput{ID: "i1", Identity: owner, PayloadHash: "h1", Mode: InputQueue})
	if err != nil || !join.Join {
		t.Fatalf("join=%+v err=%v", join, err)
	}
	if _, err := coordinator.Admit(context.Background(), AdmittedInput{ID: "i1", Identity: owner, PayloadHash: "changed", Mode: InputQueue}); !errors.Is(err, ErrInputConflict) {
		t.Fatalf("conflicting retry=%v", err)
	}
	steerOwner := owner
	steerOwner.RunID = "r2"
	steer, err := coordinator.Admit(context.Background(), AdmittedInput{ID: "i2", Identity: steerOwner, PayloadHash: "h2", Mode: InputSteer})
	if err != nil || steer.Input.State != InputSteering || steer.Input.SteersInputID != "i1" {
		t.Fatalf("steer=%+v err=%v", steer, err)
	}
	_, promoted, err := coordinator.Finish(context.Background(), owner, "i1", first.Input.Revision, InputCompleted, "")
	if err != nil || promoted == nil || promoted.ID != "i2" || promoted.State != InputRunning {
		t.Fatalf("promoted=%+v err=%v", promoted, err)
	}
}

func TestCanonicalAdmissionProjectsUnresolvedEffectsBeforeProvider(t *testing.T) {
	ledger := NewMemoryAttemptLedger()
	owner := Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}
	attempt, _, err := ledger.Admit(context.Background(), Attempt{ID: "a", Identity: owner, Tool: ToolRef{Name: "write", Version: "v1"}, PolicyVersion: "p", CatalogVersion: "c", SchemaDigest: "schema", ArgsHash: "args", TargetHash: "target", IdempotencyKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err = ledger.Start(context.Background(), owner, attempt.ID, attempt.Revision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.Settle(context.Background(), owner, attempt.ID, attempt.Revision, AttemptSettlement{State: AttemptIndeterminate, ReconcileAction: "inspect", At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := (AdmissionEnvelopeBuilder{Attempts: ledger}).Build(context.Background(), owner, "i", "p", "c", "ctx", "harness")
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ReadyForProvider() || len(envelope.Reconciliation) != 1 || envelope.AdmissionDigest == "" {
		t.Fatalf("envelope=%+v", envelope)
	}
}

func TestSessionCoordinatorGlobalQueuePromotesAcrossSessions(t *testing.T) {
	store := NewMemoryAdmissionStore()
	coordinator, _ := NewSessionCoordinator(store, 1)
	firstOwner := Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	secondOwner := Identity{PrincipalID: "u2", SessionID: "s2", RunID: "r2"}
	first, _ := coordinator.Admit(context.Background(), AdmittedInput{ID: "i1", Identity: firstOwner, PayloadHash: "h1", Mode: InputQueue})
	second, _ := coordinator.Admit(context.Background(), AdmittedInput{ID: "i2", Identity: secondOwner, PayloadHash: "h2", Mode: InputQueue})
	if second.Input.State != InputQueued {
		t.Fatalf("second=%+v", second)
	}
	_, promoted, err := coordinator.Finish(context.Background(), firstOwner, "i1", first.Input.Revision, InputCompleted, "")
	if err != nil || promoted == nil || promoted.Identity != secondOwner {
		t.Fatalf("promoted=%+v err=%v", promoted, err)
	}
}

func TestSessionCoordinatorPersistsSafeBoundaryPriority(t *testing.T) {
	store := NewMemoryAdmissionStore()
	coordinator, _ := NewSessionCoordinator(store, 1)
	owner := Identity{PrincipalID: "u", SessionID: "s", RunID: "r1"}
	first, err := coordinator.Admit(context.Background(), AdmittedInput{ID: "i1", Identity: owner, PayloadHash: "h1", Mode: InputQueue})
	if err != nil {
		t.Fatal(err)
	}
	nextOwner := owner
	nextOwner.RunID = "r2"
	next, err := coordinator.Admit(context.Background(), AdmittedInput{ID: "i2", Identity: nextOwner, PayloadHash: "h2", Mode: InputSteer, Priority: InputPriorityNow})
	if err != nil {
		t.Fatal(err)
	}
	if next.InterruptID != first.Input.ID || next.Input.Priority != InputPriorityNow || next.Input.State != InputSteering {
		t.Fatalf("decision=%+v", next)
	}
	if _, _, err := coordinator.Finish(context.Background(), owner, first.Input.ID, first.Input.Revision, InputCompleted, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRunStoreSeparatesExecutionAndDelivery(t *testing.T) {
	store := NewMemoryRunStore()
	owner := Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	run, err := store.Create(context.Background(), Run{ID: "r1", InputID: "i1", Identity: owner})
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.Update(context.Background(), owner, run.ID, run.Revision, RunSucceeded, DeliveryPending, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.Update(context.Background(), owner, run.ID, run.Revision, RunSucceeded, DeliveryDelivered, "", 3)
	if err != nil || run.Execution != RunSucceeded || run.Delivery != DeliveryDelivered {
		t.Fatalf("run=%+v err=%v", run, err)
	}
}
