package research

import (
	"context"
	"testing"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

func TestEvidenceGraphFindsGapsContradictionsAndPinsSnapshots(t *testing.T) {
	owner := execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}
	graph := Graph{
		ID: "research", Owner: owner,
		Sources:  []SourceSnapshot{{ID: "source", Locator: "https://example.invalid", Digest: "sha256:source", ContentRef: "blob:1", CapturedAt: time.Now()}},
		Evidence: []Evidence{{ID: "e1", SourceSnapshotID: "source", Locator: "p1", ExcerptHash: "sha256:e1", Quality: .9}},
		Claims:   []Claim{{ID: "c1", StatementHash: "sha256:c1"}, {ID: "c2", StatementHash: "sha256:c2"}},
		Edges:    []ClaimEvidence{{ClaimID: "c1", EvidenceID: "e1", Relation: RelationSupports, Strength: .9}, {ClaimID: "c1", EvidenceID: "e1", Relation: RelationContradicts, Strength: .8}},
	}
	store := NewMemoryStore()
	stored, err := store.Create(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	assessment := stored.Assess(.5)
	if assessment.Coverage != .5 || len(assessment.UnsupportedClaims) != 1 || len(assessment.ContradictedClaims) != 1 {
		t.Fatalf("assessment=%+v", assessment)
	}
	graph.Revision = stored.Revision
	graph.Sources[0].Digest = "sha256:changed"
	if _, err := store.CompareAndSwap(context.Background(), owner, graph.ID, stored.Revision, graph); err == nil {
		t.Fatal("source snapshot mutation accepted")
	}
}
