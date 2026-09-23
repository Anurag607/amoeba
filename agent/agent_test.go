package agent

import (
	"context"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/policy"
)

func TestBaseAgentExecutesOnlyThroughSealedDispatcher(t *testing.T) {
	base := NewBase("test")
	called := false
	if err := base.Register(execution.ToolSpec{
		Ref: execution.ToolRef{Name: "read", Version: "v1"}, SchemaDigest: "sha256:read-v1",
		Validate: func(string) error { return nil }, Execute: func(context.Context, string) (string, error) { called = true; return "ok", nil },
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := policy.NewSnapshot("p1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{{Action: "read", Effect: policy.EffectAllow, ReasonCode: "allowed"}}})
	admitted, err := base.Seal(Admission{CatalogVersion: "c1", Policy: snapshot, Ledger: execution.NewMemoryAttemptLedger()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := execution.WithIdentity(context.Background(), execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"})
	if _, err := admitted.ExecuteTool(ctx, "read", `{}`); err == nil || called {
		t.Fatal("tool executed without invocation metadata")
	}
	ctx, _ = WithToolInvocation(ctx, ToolInvocation{AttemptID: "a1"})
	if _, err := admitted.ExecuteTool(ctx, "read", `{}`); err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}
