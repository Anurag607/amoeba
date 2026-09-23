package execution

import (
	"context"
	"testing"
)

func TestToolRoundRequiresSettledMandatoryWork(t *testing.T) {
	ctx := context.Background()
	owner := Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}
	store := NewMemoryToolRoundStore()
	round, _, err := store.Create(ctx, ToolRound{ID: "round", Identity: owner, PolicyVersion: "p1", CatalogVersion: "c1", Calls: []ToolCallRecord{
		{ID: "required", Tool: ToolRef{Name: "write", Version: "1"}, Required: true},
		{ID: "optional", Tool: ToolRef{Name: "search", Version: "1"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	round, err = store.Transition(ctx, owner, round.ID, round.Revision, "required", ToolCallOmitted, "", &ToolOmission{ReasonCode: "capability.unavailable", Safe: false})
	if err != nil {
		t.Fatal(err)
	}
	round, err = store.Transition(ctx, owner, round.ID, round.Revision, "optional", ToolCallOmitted, "", &ToolOmission{ReasonCode: "not_needed", Safe: true})
	if err != nil {
		t.Fatal(err)
	}
	if assessment := round.Assess(); assessment.Converged || len(assessment.Blockers) != 1 {
		t.Fatalf("assessment=%+v", assessment)
	}

	second, _, err := store.Create(ctx, ToolRound{ID: "round-2", Identity: owner, PolicyVersion: "p1", CatalogVersion: "c1", Calls: []ToolCallRecord{{ID: "required", Tool: ToolRef{Name: "write", Version: "1"}, Required: true}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err = store.Transition(ctx, owner, second.ID, second.Revision, "required", ToolCallOmitted, "", &ToolOmission{ReasonCode: "superseded_by_equivalent_effect", Safe: true})
	if err != nil || !second.Assess().Converged {
		t.Fatalf("round=%+v err=%v", second, err)
	}
}
