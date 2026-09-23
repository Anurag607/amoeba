package policy

import (
	"reflect"
	"testing"
)

func TestDenyOverridesAllowAndApproval(t *testing.T) {
	snapshot, err := NewSnapshot("v1",
		RuleSet{Layer: LayerGlobal, Rules: []Rule{{Action: "read_*", Effect: EffectAllow, ReasonCode: "global.read"}}},
		RuleSet{Layer: LayerExpert, Rules: []Rule{{Action: "read_secret", Effect: EffectApprove, ReasonCode: "expert.approval"}}},
		RuleSet{Layer: LayerWorkspace, Rules: []Rule{{Action: "read_secret", Effect: EffectDeny, ReasonCode: "workspace.deny", Immutable: true}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	decision := snapshot.Decide("read_secret", "")
	if decision.Effect != EffectDeny || decision.ReasonCode != "workspace.deny" || !decision.Immutable {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if got, want := snapshot.AllowedActions([]string{"write_file", "read_file", "read_secret"}), []string{"read_file"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered actions got %v want %v", got, want)
	}
}

func TestNoMatchFailsClosed(t *testing.T) {
	snapshot, err := NewSnapshot("v1", RuleSet{Layer: LayerGlobal})
	if err != nil {
		t.Fatal(err)
	}
	if decision := snapshot.Decide("unknown", ""); decision.Effect != EffectDeny || decision.ReasonCode != "policy.no_matching_allow" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}
