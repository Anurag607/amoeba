package moetools

import (
	"context"
	"strings"
	"testing"

	"github.com/anurgosw/agentic-moe/agent"
	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/policy"
	"github.com/anurgosw/agentic-moe/skills"
)

func TestRegisteredSkillToolsUseSealedPolicyAndStrictArguments(t *testing.T) {
	registry := skills.NewRegistry()
	registry.Register(&skills.Skill{ID: "review", Name: "Review", Scope: "project", Content: "inspect carefully"})
	base := agent.NewBase("skills")
	if err := Register(&base, registry); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := policy.NewSnapshot("p1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{
		{Action: NameLoadSkill, Effect: policy.EffectAllow, ReasonCode: "load.allowed"},
		{Action: "skill.review", Effect: policy.EffectAllow, ReasonCode: "skill.allowed"},
	}})
	admitted, err := base.Seal(agent.Admission{CatalogVersion: "c1", Policy: snapshot, Ledger: execution.NewMemoryAttemptLedger()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := execution.WithIdentity(context.Background(), execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"})
	ctx, _ = agent.WithToolInvocation(ctx, agent.ToolInvocation{AttemptID: "a1"})
	out, err := admitted.ExecuteTool(ctx, NameLoadSkill, `{"skill_id":"review"}`)
	if err != nil || !strings.Contains(out, "inspect carefully") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	ctx2, _ := execution.WithIdentity(context.Background(), execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"})
	ctx2, _ = agent.WithToolInvocation(ctx2, agent.ToolInvocation{AttemptID: "a2"})
	if _, err := admitted.ExecuteTool(ctx2, NameLoadSkill, `{"skill_id":"review","extra":true}`); err == nil {
		t.Fatal("unknown argument field accepted")
	}
}
