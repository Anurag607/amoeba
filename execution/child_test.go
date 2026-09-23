package execution

import (
	"context"
	"testing"
)

func TestChildRunUsesRevisionedTerminalTransitions(t *testing.T) {
	store := NewMemoryChildRunStore()
	identity := Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	ctx, _ := WithIdentity(context.Background(), identity)
	run := ChildRun{ID: "c1", ParentRunID: "r1", ExpertID: "research", Identity: identity}
	if err := store.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	running, err := store.Transition(ctx, "c1", 1, ChildRunning, "", "")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Transition(ctx, "c1", running.Revision, ChildCompleted, "artifact:c1", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != ChildCompleted || completed.ArtifactRef == "" {
		t.Fatalf("bad child result: %+v", completed)
	}
	if _, err := store.Transition(ctx, "c1", completed.Revision, ChildFailed, "", "late"); err == nil {
		t.Fatal("terminal child transitioned twice")
	}
	otherCtx, _ := WithIdentity(context.Background(), Identity{PrincipalID: "u2", SessionID: "s2", RunID: "r2"})
	if _, ok := store.Get(otherCtx, "c1"); ok {
		t.Fatal("other owner read child run")
	}
}
