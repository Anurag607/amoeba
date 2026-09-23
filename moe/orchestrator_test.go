package moe

import (
	"strings"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/policy"
	"github.com/anurgosw/agentic-moe/skills"
	"github.com/tmc/langchaingo/llms"
)

func TestDelegatedPlanIsFailClosedAndHonorsAdmission(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ExpertDefinition{ID: "code", Name: "Code", ToolSetName: "code"})
	orch := New(Config{
		Experts: reg,
		ToolResolver: func(string) []llms.Tool {
			return []llms.Tool{testTool("read_file"), testTool("write_file")}
		},
		Tiers: TierMapping{ModelTierBalanced: "heuristic-model"},
	})
	plan, err := orch.PlanForExpert("code")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tools) != 0 {
		t.Fatalf("zero delegated ceiling exposed tools: %v", plan.ToolNames)
	}
	plan, err = orch.PlanForExpertWithAdmission("code", DelegationAdmission{
		ToolCeiling:   NewCapabilityCeiling("read_file"),
		ToolCallModel: "owner-model", SynthesisModel: "owner-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ToolNames) != 1 || plan.ToolNames[0] != "read_file" {
		t.Fatalf("capability ceiling failed: %v", plan.ToolNames)
	}
	if plan.Options.ToolCallModel != "owner-model" || plan.Options.SynthesisModel != "owner-model" {
		t.Fatalf("admitted model did not win: %+v", plan.Options)
	}
	unrestricted, err := orch.PlanForExpertUnrestricted("code")
	if err != nil {
		t.Fatal(err)
	}
	if len(unrestricted.Tools) != 2 {
		t.Fatalf("explicit unrestricted plan got %d tools", len(unrestricted.Tools))
	}
}

func TestPlanPinsManifestAndFiltersExpertsSkillsAndCentralTools(t *testing.T) {
	experts := NewRegistry()
	experts.Register(&ExpertDefinition{ID: "allowed", Name: "Allowed", SkillIDs: []string{"good", "bad"}, ToolSetName: "set"})
	experts.Register(&ExpertDefinition{ID: "hidden", Name: "Hidden"})
	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(&skills.Skill{ID: "good", Name: "Good", Scope: "project", Content: "good"})
	skillRegistry.Register(&skills.Skill{ID: "bad", Name: "Bad", Scope: "project", Content: "bad"})
	snapshot, _ := policy.NewSnapshot("p1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{
		{Action: "expert.allowed", Effect: policy.EffectAllow, ReasonCode: "allowed"},
		{Action: "expert.hidden", Effect: policy.EffectDeny, ReasonCode: "hidden"},
		{Action: "skill.good", Effect: policy.EffectAllow, ReasonCode: "allowed"},
		{Action: "skill.bad", Effect: policy.EffectDeny, ReasonCode: "hidden"},
		{Action: "read", Effect: policy.EffectAllow, ReasonCode: "allowed"},
		{Action: "consult_expert", Effect: policy.EffectAllow, ReasonCode: "allowed"},
		{Action: "delegate_to_expert", Effect: policy.EffectAllow, ReasonCode: "allowed"},
	}})
	orch := New(Config{Experts: experts, Skills: skillRegistry, Policy: &snapshot, CatalogVersion: "c1", ContextDigest: "ctx1",
		CapabilityResolver: func(string) []ToolBinding {
			return []ToolBinding{{Tool: testTool("read"), Ref: execution.ToolRef{Name: "read", Version: "v1"}, SchemaDigest: "sha256:read"}}
		},
	})
	plan, err := orch.PlanForExpertUnrestricted("allowed")
	if err != nil {
		t.Fatal(err)
	}
	if plan.PolicyVersion != "p1" || plan.CatalogVersion != "c1" || plan.ContextDigest != "ctx1" || len(plan.ToolBindings) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if len(plan.LoadedSkills) != 1 || plan.LoadedSkills[0] != "good" || strings.Contains(plan.SkillContext, "Bad") {
		t.Fatalf("skills=%v context=%q", plan.LoadedSkills, plan.SkillContext)
	}
	central, err := orch.PlanCentral()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(central.SystemPromptAugmentation, "Hidden") {
		t.Fatalf("hidden expert disclosed: %s", central.SystemPromptAugmentation)
	}
}

func TestDelegatedPlanFiltersDiscoveryByPolicyAndCopiesChildProfile(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ExpertDefinition{ID: "code", Name: "Code", ToolSetName: "code"})
	orch := New(Config{Experts: reg, ToolResolver: func(string) []llms.Tool {
		return []llms.Tool{testTool("read_file"), testTool("write_file")}
	}})
	snapshot, err := policy.NewSnapshot("policy-v1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{
		{Action: "expert.code", Effect: policy.EffectAllow, ReasonCode: "expert.allowed"},
		{Action: "read_file", Effect: policy.EffectAllow, ReasonCode: "read.allowed"},
		{Action: "write_file", Effect: policy.EffectDeny, ReasonCode: "write.denied"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	sources := []string{"workspace"}
	plan, err := orch.PlanForExpertWithAdmission("code", DelegationAdmission{
		ToolCeiling: UnrestrictedCapabilityCeiling(), Policy: &snapshot,
		ChildProfile: ChildProfile{AllowNetwork: true, ContextSources: sources},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ToolNames) != 1 || plan.ToolNames[0] != "read_file" {
		t.Fatalf("policy-filtered tools = %v", plan.ToolNames)
	}
	sources[0] = "mutated"
	if plan.ChildProfile.ContextSources[0] != "workspace" {
		t.Fatal("plan aliased caller-owned child profile")
	}
}

type testAdaptivePlanner struct{ calls int }

func (p *testAdaptivePlanner) Recommend(input AdaptiveInput, lookup StatsLookup) AdaptiveDecision {
	p.calls++
	return AdaptiveDecision{ResponseTokenBudget: 900, MaxIterations: 7, EnableChunking: true, Reason: "history"}
}

func TestAdaptivePlanningCanBeFullyDisabled(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ExpertDefinition{ID: "code", Name: "Code"})
	planner := &testAdaptivePlanner{}
	orch := New(Config{Experts: reg, Adaptive: planner, Learning: LearningConfig{Disabled: true}})
	plan, err := orch.PlanForExpertUnrestricted("code")
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 0 || plan.Adaptive != (AdaptiveDecision{}) {
		t.Fatalf("disabled learning performed work: calls=%d decision=%+v", planner.calls, plan.Adaptive)
	}
	orch = New(Config{Experts: reg, Adaptive: planner})
	plan, err = orch.PlanForExpertUnrestricted("code")
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 1 || plan.Options.ResponseTokenBudget != 900 || plan.Options.MaxIterations != 7 || !plan.Options.EnableChunking {
		t.Fatalf("adaptive plan not applied: calls=%d plan=%+v", planner.calls, plan)
	}
}

func testTool(name string) llms.Tool {
	return llms.Tool{Type: "function", Function: &llms.FunctionDefinition{Name: name}}
}

func TestRouterNormalizesPartialConfigAndAccumulatesContext(t *testing.T) {
	reg := NewRegistry()
	expert := &ExpertDefinition{ID: "ops", Name: "Ops", Keywords: []string{"trace", "build"}}
	reg.Register(expert)
	router := NewRouter(reg, RouterConfig{MinConfidence: 0.9})
	if router.cfg.MinConfidence != 0.9 || router.cfg.KeywordWeight != 1.0 {
		t.Fatalf("partial config was discarded: %+v", router.cfg)
	}
	score := router.scoreExpert(expert, "", RoutingContext{HasTraceContext: true, HasBuildContext: true})
	if score != 0.8 {
		t.Fatalf("context boosts were not accumulated: got %.1f want 0.8", score)
	}
}
