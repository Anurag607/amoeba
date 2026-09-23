package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
)

func TestMutableWorkspaceLeaseUsesCanonicalRootAndOwnerCAS(t *testing.T) {
	manager := NewMemoryManager()
	owner := execution.Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	first := Lease{ID: "l1", Workspace: "repo-one", Root: root, Mode: ModeMutable, Owner: owner}
	if err := manager.Acquire(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := Lease{ID: "l2", Workspace: "different-label", Root: alias, Mode: ModeReadOnly, Owner: owner}
	if err := manager.Acquire(context.Background(), second); err == nil {
		t.Fatal("canonical-root alias bypassed exclusive lease")
	}
	stored, ok, err := manager.Get(context.Background(), "l1", owner)
	if err != nil || !ok || stored.CanonicalRoot == "" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	other := execution.Identity{PrincipalID: "u2", SessionID: "s2", RunID: "r2"}
	if err := manager.Release(context.Background(), "l1", other, stored.Revision); err == nil {
		t.Fatal("wrong owner released lease")
	}
	if err := manager.Release(context.Background(), "l1", owner, stored.Revision+1); err == nil {
		t.Fatal("stale revision released lease")
	}
	if err := manager.Release(context.Background(), "l1", owner, stored.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentSealBindsReproducibleConfiguration(t *testing.T) {
	environment, err := (Environment{
		Placement: PlacementContainer, WorkspaceLeaseID: "lease-1", ImageDigest: "sha256:image",
		Repositories: []Repository{{URL: "https://example.invalid/repo", Revision: "abc123", Path: "src"}},
		Network:      NetworkNone, CleanupPolicy: "remove",
	}).Seal()
	if err != nil {
		t.Fatal(err)
	}
	if environment.Digest == "" || environment.Version != 1 {
		t.Fatalf("environment=%+v", environment)
	}
	environment.Repositories[0].Revision = "changed"
	if err := environment.Validate(); err == nil {
		t.Fatal("mutated environment retained valid digest")
	}
}
