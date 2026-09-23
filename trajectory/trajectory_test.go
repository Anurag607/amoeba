package trajectory

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryStoreIsMonotonicIdempotentAndScoped(t *testing.T) {
	store := NewMemoryStore()
	first, err := store.Append(context.Background(), Event{ID: "e1", SessionID: "s1", RunID: "r1", Type: "started"})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.Append(context.Background(), Event{ID: "e1", SessionID: "s1", RunID: "r1", Type: "started"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := store.Append(context.Background(), Event{ID: "e2", SessionID: "s1", RunID: "r1", Type: "completed"})
	_, _ = store.Append(context.Background(), Event{ID: "other", SessionID: "s2", RunID: "r1", Type: "started"})
	if first.Sequence != 1 || duplicate.Type != "started" || second.Sequence != 2 {
		t.Fatalf("bad sequence/idempotency")
	}
	if _, err := store.Append(context.Background(), Event{ID: "e1", SessionID: "s1", RunID: "r1", Type: "different"}); err == nil {
		t.Fatal("conflicting event identity was accepted")
	}
	replay, err := store.Replay(context.Background(), "s1", "r1", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].ID != "e2" {
		t.Fatalf("bad replay: %+v", replay)
	}
}

func TestReplayAcknowledgementRetentionAndDegradedState(t *testing.T) {
	store := NewMemoryStoreWithRetention(2)
	for i, id := range []string{"e1", "e2", "e3"} {
		if _, err := store.Append(context.Background(), Event{ID: id, SessionID: "s", RunID: "r", Type: "step", Data: map[string]interface{}{"i": i}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Replay(context.Background(), "s", "r", 0, 10); !errors.Is(err, ErrCursorExpired) {
		t.Fatalf("expired cursor was replayed: %v", err)
	}
	state, err := store.Ack(context.Background(), "s", "r", 2)
	if err != nil || state.EarliestSequence != 2 || state.HeadSequence != 3 || state.AckedSequence != 2 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if err := store.SetDegraded(context.Background(), "s", "r", "storage unavailable"); err != nil {
		t.Fatal(err)
	}
	state, _ = store.State(context.Background(), "s", "r")
	if state.Reliability != ReliabilityDegraded || state.LastError == "" {
		t.Fatalf("state=%+v", state)
	}
}

func TestStoredEventDoesNotAliasNestedCallerData(t *testing.T) {
	store := NewMemoryStore()
	nested := map[string]interface{}{"value": "original"}
	event, err := store.Append(context.Background(), Event{ID: "e1", SessionID: "s", RunID: "r", Type: "step",
		Data: map[string]interface{}{"nested": nested}})
	if err != nil {
		t.Fatal(err)
	}
	nested["value"] = "mutated"
	event.Data["nested"].(map[string]interface{})["value"] = "also-mutated"
	replayed, err := store.Replay(context.Background(), "s", "r", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := replayed[0].Data["nested"].(map[string]interface{})["value"]
	if got != "original" {
		t.Fatalf("stored event was mutated through alias: %v", got)
	}
}
