package execution

import (
	"context"
	"testing"
)

func TestTaskMailboxIsCursorDrivenAndIdempotent(t *testing.T) {
	owner := Identity{PrincipalID: "u", SessionID: "s", RunID: "root"}
	mailbox := NewMemoryTaskMailbox()
	message := TaskMessage{ID: "m1", Owner: owner, SenderTaskID: "lead", RecipientTaskID: "worker", PayloadHash: "sha256:x"}
	first, created, err := mailbox.Append(context.Background(), message)
	if err != nil || !created || first.Sequence != 1 {
		t.Fatalf("message=%+v created=%v err=%v", first, created, err)
	}
	if _, created, err := mailbox.Append(context.Background(), message); err != nil || created {
		t.Fatalf("duplicate created=%v err=%v", created, err)
	}
	items, err := mailbox.Replay(context.Background(), owner, "worker", 0, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	state, err := mailbox.Ack(context.Background(), owner, "worker", 1)
	if err != nil || state.AckedSequence != 1 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}
