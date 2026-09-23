package workspace

import (
	"context"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
)

func TestTerminalControllerPinsOwnershipAndGeneration(t *testing.T) {
	ctx := context.Background()
	identity := execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}
	store := NewMemoryTerminalStore()
	lease, err := store.Create(ctx, TerminalLease{ID: "pty", Identity: identity, Rows: 24, Columns: 80})
	if err != nil {
		t.Fatal(err)
	}
	controller := TerminalController{Store: store}
	lease, err = controller.Handoff(ctx, identity, lease.ID, lease.Revision, TerminalAgent)
	if err != nil || lease.Owner != TerminalAgent || lease.Generation != 2 {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}
	if _, err := controller.Resize(ctx, identity, lease.ID, lease.Revision, TerminalUser, 30, 100); err == nil {
		t.Fatal("non-owner resized terminal")
	}
	lease, err = controller.Disconnect(ctx, identity, lease.ID, lease.Revision)
	if err != nil || lease.Owner != TerminalHost || lease.Generation != 3 {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}
}
