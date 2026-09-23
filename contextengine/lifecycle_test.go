package contextengine

import (
	"context"
	"strings"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
)

func TestLifecyclePinsAndCASCommitsTurnSnapshot(t *testing.T) {
	source := &testSource{key: "runtime", revision: "1", content: "one"}
	registry := NewRegistry()
	_ = registry.Register(source)
	lifecycle := Lifecycle{Engine: Engine{Registry: registry}, Store: NewMemorySnapshotStore()}
	owner := execution.Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	turn, err := lifecycle.BeginTurn(context.Background(), owner, "s1", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Record.Digest == "" || turn.Next.Epoch != 1 {
		t.Fatalf("turn=%+v", turn)
	}
	source.revision, source.content = "2", "two"
	next, err := lifecycle.BeginTurn(context.Background(), owner, "s1", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := lifecycle.Commit(context.Background(), next)
	if err != nil || committed.Snapshot.Sources["runtime"].Revision != "2" {
		t.Fatalf("record=%+v err=%v", committed, err)
	}
	if _, err := lifecycle.Commit(context.Background(), next); err == nil {
		t.Fatal("stale turn committed twice")
	}
}

func TestOutputManagerUsesCentralSpillBoundary(t *testing.T) {
	spill := &testSpillStore{}
	manager := OutputManager{MaxBytes: 16, Spill: spill}
	frame, err := manager.Bound(context.Background(), "tool:1", "v1", ClassToolResult, TrustUntrusted, strings.Repeat("x", 64))
	if err != nil {
		t.Fatal(err)
	}
	if frame.SpillRef == "" || len(frame.Content) > 16 || len(spill.content) != 64 {
		t.Fatalf("frame=%+v", frame)
	}
}
