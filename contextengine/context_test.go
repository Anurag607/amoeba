package contextengine

import (
	"context"
	"errors"
	"testing"
	"unicode/utf8"
)

type testSource struct {
	key, revision, content string
	err                    error
}

type testSpillStore struct {
	content string
	err     error
}

func (s *testSpillStore) Put(_ context.Context, _ string, content string) (string, error) {
	s.content = content
	return "spill:1", s.err
}

func (s *testSource) Key() string                              { return s.key }
func (s *testSource) Revision(context.Context) (string, error) { return s.revision, s.err }
func (s *testSource) Resolve(context.Context) ([]Frame, error) {
	return []Frame{{Class: ClassRuntime, Trust: TrustAuthoritative, Content: s.content}}, s.err
}

func TestReconcileOrdersChangesAndRetainsUnavailableState(t *testing.T) {
	a := &testSource{key: "a", revision: "1", content: "alpha"}
	b := &testSource{key: "b", revision: "1", content: "beta"}
	reg := NewRegistry()
	_ = reg.Register(b)
	_ = reg.Register(a)
	engine := Engine{Registry: reg}
	first, updates, err := engine.Reconcile(context.Background(), Snapshot{}, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updates[0].SourceKey != "a" || updates[1].SourceKey != "b" {
		t.Fatalf("updates not deterministic: %+v", updates)
	}
	a.err = errors.New("temporary")
	second, updates, err := engine.Reconcile(context.Background(), first, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].Kind != UpdateUnavailable || second.Sources["a"].Frames[0].Content != "alpha" {
		t.Fatalf("unavailable source did not retain admitted state: %+v %+v", updates, second)
	}
	third, updates, err := engine.Reconcile(context.Background(), second, "ws2")
	if err == nil || third.Sources != nil || updates != nil {
		t.Fatalf("workspace move reused unavailable baseline")
	}
}

func TestRebaselineAdvancesEpoch(t *testing.T) {
	in := Snapshot{Epoch: 4, Sources: map[string]SourceState{"a": {Revision: "1"}}}
	out := Rebaseline(in)
	if out.Epoch != 5 || Digest(out) != Digest(in) {
		t.Fatalf("bad rebaseline: %+v", out)
	}
}

func TestOversizedContextSpillsWithoutLosingCompleteContent(t *testing.T) {
	source := &testSource{key: "large", revision: "1", content: "αβγδεζηθικλμνξοπρστυφχψω"}
	registry := NewRegistry()
	_ = registry.Register(source)
	spill := &testSpillStore{}
	snapshot, _, err := (Engine{Registry: registry, MaxFrameBytes: 20, Spill: spill}).Reconcile(context.Background(), Snapshot{}, "ws")
	if err != nil {
		t.Fatal(err)
	}
	frame := snapshot.Sources["large"].Frames[0]
	if spill.content != source.content || frame.SpillRef != "spill:1" || len(frame.Content) > 20 || !utf8.ValidString(frame.Content) {
		t.Fatalf("bad spill result: frame=%+v stored=%q", frame, spill.content)
	}
}
