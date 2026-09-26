package workspace

import (
	"context"
	"testing"

	"github.com/Anurag607/amoeba/execution"
)

type testWorktreeDriver struct {
	root    string
	cleaned bool
}

func (d *testWorktreeDriver) Create(context.Context, WorktreeRequest) (string, error) {
	return d.root, nil
}
func (d *testWorktreeDriver) Cleanup(context.Context, string) error { d.cleaned = true; return nil }

func TestManagedWorktreePersistsLeaseRecoversAndCleansUp(t *testing.T) {
	owner := execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}
	driver := &testWorktreeDriver{root: t.TempDir()}
	service := ManagedWorktrees{Driver: driver, Store: NewMemoryWorktreeStore(), Leases: NewMemoryManager()}
	lease, err := service.Create(context.Background(), WorktreeRequest{ID: "w1", Owner: owner, Source: "repo", Revision: "abc", Path: "work"})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := service.Recover(context.Background(), owner)
	if err != nil || len(recovered) != 1 || recovered[0].ID != lease.ID {
		t.Fatalf("recovered=%+v err=%v", recovered, err)
	}
	item, ok, err := service.Store.Get(context.Background(), owner, "w1")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err := service.Cleanup(context.Background(), "w1", owner, item.Revision); err != nil {
		t.Fatal(err)
	}
	if !driver.cleaned {
		t.Fatal("worktree driver did not clean up")
	}
}
