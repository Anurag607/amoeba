package moe

import (
	"fmt"

	"github.com/Anurag607/amoeba/execution"
	"github.com/Anurag607/amoeba/log"
	"github.com/Anurag607/amoeba/policy"
	"github.com/Anurag607/amoeba/skills"

	"github.com/tmc/langchaingo/llms"
)

// ToolResolver maps a tool-set name (e.g. "observability", "code") to the
// concrete []llms.Tool the consumer wants exposed for that expert. The
// resolver is supplied by the consumer at orchestrator construction.
type ToolResolver func(toolSetName string) []llms.Tool

type ToolBinding struct {
	Tool         llms.Tool
	Ref          execution.ToolRef
	SchemaDigest string
}

type CapabilityResolver func(toolSetName string) []ToolBinding

// LoopOptions are the per-plan execution caps. Consumers feed these into
// their own agentic loop; the orchestrator does not run the loop itself.
type LoopOptions struct {
	ToolCallModel            string
	SynthesisModel           string
	ResponseTokenBudget      int
	EnableChunking           bool
	MaxIterations            int
	MaxToolCalls             int
	MinToolCallsBeforeFinish int
}

// DelegationAdmission is the host-authorized execution envelope for a child.
// Model fields let the host preserve an owner/user model selection instead of
// allowing heuristic tier routing to silently switch delegated credentials.
type DelegationAdmission struct {
	ToolCeiling    CapabilityCeiling
	Policy         *policy.Snapshot
	ChildProfile   ChildProfile
	ToolCallModel  string
	SynthesisModel string
	ContextDigest  string
	CatalogVersion string
}

// Plan is the orchestrator's output: everything the caller needs to compose
// a system prompt + run an agentic loop for the selected expert.
type Plan struct {
	Expert         *ExpertDefinition
	Selection      *Selection
	SkillContext   string
	Tools          []llms.Tool
	ToolNames      []string
	Options        LoopOptions
	Tier           TierDecision
	Adaptive       AdaptiveDecision
	ChildProfile   ChildProfile
	LoadedSkills   []string
	OmittedSkills  []string
	PolicyVersion  string
	CatalogVersion string
	ContextDigest  string
	ToolBindings   map[string]ToolBinding

	// SystemPromptAugmentation is the text the caller appends to its base
	// system prompt (skills + expert-specific prompt + optional central
	// network description for central plans).
	SystemPromptAugmentation string

	// IsCentral is true for plans produced by PlanCentral.
	IsCentral bool
}

// Config bundles the inputs the orchestrator needs to build plans.
type Config struct {
	Experts      *Registry
	Skills       *skills.Registry
	Router       *Router
	ToolResolver ToolResolver
	// CapabilityResolver is the authoritative versioned discovery source.
	// ToolResolver remains a migration path for unversioned callers.
	CapabilityResolver      CapabilityResolver
	Policy                  *policy.Snapshot
	CatalogVersion          string
	ContextDigest           string
	ProviderForModel        map[string]string
	EffectiveContextWindows map[string]int
	Classifier              ComplexityClassifier
	Tiers                   TierMapping
	Emit                    EventEmitter
	DefaultOptions          LoopOptions
	Learning                LearningConfig
	Adaptive                AdaptivePlanner
	// DelegatedToolCeiling is applied by PlanForExpert. Its zero value denies
	// all tools, requiring the host to authorize delegated capabilities.
	DelegatedToolCeiling CapabilityCeiling
	// MaxInjectedSkillTokens bounds static skill prompt augmentation. Zero uses
	// a conservative 4096-token default; omitted skills remain loadable via
	// load_skill when the host exposes that tool.
	MaxInjectedSkillTokens int

	// CentralToolSet is the tool-set name used when planning a central run.
	// The orchestrator appends consult_expert + delegate_to_expert tool
	// definitions automatically on top of whatever the resolver returns.
	CentralToolSet string
}

// Orchestrator builds Plans. It does not run agentic loops; that's the
// caller's job.
type Orchestrator struct {
	cfg Config
}

// New constructs an Orchestrator. Defaults: empty classifier (uses internal
// defaults), no-op emitter, DefaultRouterConfig if Router is nil.
func New(cfg Config) *Orchestrator {
	if cfg.Router == nil && cfg.Experts != nil {
		cfg.Router = NewRouter(cfg.Experts, DefaultRouterConfig())
	}
	if cfg.Classifier == (ComplexityClassifier{}) {
		cfg.Classifier = DefaultClassifier()
	}
	if cfg.Emit == nil {
		cfg.Emit = func(Event) {}
	}
	if !cfg.Learning.Disabled && cfg.Adaptive == nil {
		cfg.Adaptive = DefaultAdaptivePlanner{}
	}
	if cfg.MaxInjectedSkillTokens <= 0 {
		cfg.MaxInjectedSkillTokens = 4096
	}
	return &Orchestrator{cfg: cfg}
}

// Plan routes the query to an expert and returns the execution plan.
func (o *Orchestrator) Plan(query string, ctx RoutingContext) (*Plan, error) {
	if o.cfg.Experts == nil || len(o.cfg.Experts.IDs()) == 0 {
		return nil, fmt.Errorf("moe: no experts registered")
	}
	sel := o.cfg.Router.Route(query, ctx)
	if sel == nil || sel.Primary == nil {
		return nil, fmt.Errorf("moe: routing failed")
	}
	if o.cfg.Policy != nil && o.cfg.Policy.Decide("expert."+string(sel.Primary.ID), "").Effect == policy.EffectDeny {
		return nil, fmt.Errorf("moe: selected expert %q denied by admitted policy", sel.Primary.ID)
	}
	o.cfg.Emit(Event{
		Type:       EventTypeRouting,
		ExpertID:   sel.Primary.ID,
		ExpertName: sel.Primary.Name,
		Confidence: sel.Confidences[sel.Primary.ID],
		Reasoning:  sel.Reasoning,
		Message:    "router selected " + string(sel.Primary.ID),
	})
	if sel.Primary.CanSynthesize && len(sel.Secondary) > 0 {
		o.cfg.Emit(Event{
			Type:       EventTypeSynthesis,
			ExpertID:   sel.Primary.ID,
			ExpertName: sel.Primary.Name,
			Message:    "synthesis expert engaged across multiple domains",
		})
	}
	return o.buildPlan(sel.Primary, sel, query, ctx, false, o.cfg.Policy)
}

// PlanForExpert builds a plan for a specific expert (bypasses routing).
// Useful for delegate_to_expert sub-loops or manual expert overrides.
func (o *Orchestrator) PlanForExpert(id ExpertID) (*Plan, error) {
	return o.PlanForExpertWithCeiling(id, o.cfg.DelegatedToolCeiling)
}

// PlanForExpertWithCeiling builds a delegated plan using an exact capability
// ceiling supplied at admission time.
func (o *Orchestrator) PlanForExpertWithCeiling(id ExpertID, ceiling CapabilityCeiling) (*Plan, error) {
	return o.PlanForExpertWithAdmission(id, DelegationAdmission{ToolCeiling: ceiling})
}

// PlanForExpertWithAdmission builds a child plan inside the exact model and
// capability envelope authorized by the host at root-run admission.
func (o *Orchestrator) PlanForExpertWithAdmission(id ExpertID, admission DelegationAdmission) (*Plan, error) {
	e, ok := o.cfg.Experts.Get(id)
	if !ok {
		return nil, fmt.Errorf("moe: unknown expert %q", id)
	}
	admittedPolicy := admission.Policy
	if admittedPolicy == nil {
		admittedPolicy = o.cfg.Policy
	}
	if admittedPolicy != nil && admittedPolicy.Decide("expert."+string(id), "").Effect == policy.EffectDeny {
		return nil, fmt.Errorf("moe: expert %q denied by admitted policy", id)
	}
	sel := &Selection{
		Primary:     e,
		Confidences: map[ExpertID]float64{e.ID: 1.0},
		Reasoning:   "explicit expert selection",
	}
	o.cfg.Emit(Event{
		Type:       EventTypeExpert,
		ExpertID:   e.ID,
		ExpertName: e.Name,
		Confidence: 1.0,
		Message:    "expert activated by explicit selection",
	})
	plan, err := o.buildPlan(e, sel, "", RoutingContext{}, false, admittedPolicy)
	if err != nil {
		return nil, err
	}
	plan.Tools = admission.ToolCeiling.filter(plan.Tools)
	if admission.Policy != nil {
		plan.Tools = filterToolsByPolicy(plan.Tools, *admission.Policy)
		plan.ToolBindings = filterBindings(plan.ToolBindings, *admission.Policy)
	}
	plan.ToolNames = toolNames(plan.Tools)
	plan.ChildProfile = admission.ChildProfile.clone()
	if admission.ContextDigest != "" {
		plan.ContextDigest = admission.ContextDigest
	}
	if admission.CatalogVersion != "" {
		plan.CatalogVersion = admission.CatalogVersion
	}
	if admission.Policy != nil {
		plan.PolicyVersion = admission.Policy.Version()
	}
	if admission.ToolCallModel != "" {
		plan.Options.ToolCallModel = admission.ToolCallModel
	}
	if admission.SynthesisModel != "" {
		plan.Options.SynthesisModel = admission.SynthesisModel
	}
	return plan, nil
}

func filterToolsByPolicy(tools []llms.Tool, snapshot policy.Snapshot) []llms.Tool {
	filtered := make([]llms.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool.Function == nil {
			continue
		}
		if snapshot.Decide(tool.Function.Name, "").Effect != policy.EffectDeny {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// PlanForExpertUnrestricted builds a trusted/manual expert plan. Prefer
// PlanForExpert or PlanForExpertWithCeiling for model-directed children.
func (o *Orchestrator) PlanForExpertUnrestricted(id ExpertID) (*Plan, error) {
	return o.PlanForExpertWithCeiling(id, UnrestrictedCapabilityCeiling())
}

// PlanCentral builds a plan for the central coordinator agent. The central
// agent gets consult_expert + delegate_to_expert in addition to whatever
// tools cfg.CentralToolSet resolves to.
func (o *Orchestrator) PlanCentral() (*Plan, error) {
	if o.cfg.Experts == nil || len(o.cfg.Experts.IDs()) == 0 {
		return nil, fmt.Errorf("moe: no experts registered")
	}
	tools, bindings := o.resolveTools(o.cfg.CentralToolSet)
	experts := o.visibleExperts(o.cfg.Policy)
	consult, delegate := ConsultToolFor(experts), DelegateToolFor(experts)
	tools = append(tools, consult, delegate)
	if bindings == nil {
		bindings = make(map[string]ToolBinding)
	}
	bindings["consult_expert"] = ToolBinding{Tool: consult, Ref: execution.ToolRef{Name: "consult_expert", Version: "v1"}, SchemaDigest: toolSchemaDigest(consult)}
	bindings["delegate_to_expert"] = ToolBinding{Tool: delegate, Ref: execution.ToolRef{Name: "delegate_to_expert", Version: "v1"}, SchemaDigest: toolSchemaDigest(delegate)}
	if o.cfg.Policy != nil {
		tools = filterToolsByPolicy(tools, *o.cfg.Policy)
		bindings = filterBindings(bindings, *o.cfg.Policy)
	}

	tier := o.cfg.Classifier.Classify("", RoutingContext{}, nil, ModelTierStrong)

	o.cfg.Emit(Event{
		Type:    EventTypeCentral,
		Tier:    tier.Synthesis.String(),
		Message: "central agent dispatch",
	})

	return &Plan{
		Selection: &Selection{Reasoning: "central coordinator"},
		Tools:     tools,
		ToolNames: toolNames(tools),
		Options: LoopOptions{
			ToolCallModel:            o.cfg.Tiers.Resolve(tier.ToolCall),
			SynthesisModel:           o.cfg.Tiers.Resolve(tier.Synthesis),
			MaxIterations:            o.cfg.DefaultOptions.MaxIterations,
			MaxToolCalls:             o.cfg.DefaultOptions.MaxToolCalls,
			MinToolCallsBeforeFinish: o.cfg.DefaultOptions.MinToolCallsBeforeFinish,
		},
		Tier:                     tier,
		PolicyVersion:            policyVersion(o.cfg.Policy),
		CatalogVersion:           o.cfg.CatalogVersion,
		ContextDigest:            o.cfg.ContextDigest,
		ToolBindings:             bindings,
		SystemPromptAugmentation: CentralPromptAugmentationFor(experts, o.cfg.Skills),
		IsCentral:                true,
	}, nil
}

func (o *Orchestrator) buildPlan(
	e *ExpertDefinition,
	sel *Selection,
	query string,
	ctx RoutingContext,
	isCentral bool,
	admittedPolicy *policy.Snapshot,
) (*Plan, error) {
	tools, bindings := o.resolveTools(e.ToolSetName)
	if admittedPolicy != nil {
		tools = filterToolsByPolicy(tools, *admittedPolicy)
		bindings = filterBindings(bindings, *admittedPolicy)
	}
	skillCtx := ""
	var loadedSkills, omittedSkills []string
	if o.cfg.Skills != nil {
		skillIDs := o.policySkillIDs(e.SkillIDs, admittedPolicy)
		skillCtx, loadedSkills, omittedSkills = o.cfg.Skills.AssembleContextBudget(skillIDs, o.cfg.MaxInjectedSkillTokens)
	}
	tier := o.cfg.Classifier.Classify(query, ctx, sel, e.DefaultTier)

	opts := o.cfg.DefaultOptions
	if e.MaxIterations > 0 {
		opts.MaxIterations = e.MaxIterations
	}
	if e.MaxToolCalls > 0 {
		opts.MaxToolCalls = e.MaxToolCalls
	}
	if e.MinToolCallsBeforeFinish > 0 {
		opts.MinToolCallsBeforeFinish = e.MinToolCallsBeforeFinish
	}
	if name := o.cfg.Tiers.Resolve(tier.ToolCall); name != "" {
		opts.ToolCallModel = name
	}
	if name := o.cfg.Tiers.Resolve(tier.Synthesis); name != "" {
		opts.SynthesisModel = name
	}
	adaptive := AdaptiveDecision{}
	if !o.cfg.Learning.Disabled && o.cfg.Adaptive != nil {
		adaptive = o.cfg.Adaptive.Recommend(AdaptiveInput{
			Key:                  StatsKey{Provider: o.cfg.ProviderForModel[opts.ToolCallModel], Model: opts.ToolCallModel, TaskType: string(e.ID)},
			EstimatedInputTokens: len([]rune(query)) / 4,
			EffectiveContext:     o.cfg.EffectiveContextWindows[opts.ToolCallModel],
			HasTools:             len(tools) > 0,
			Base:                 opts,
		}, o.cfg.Learning.Lookup)
		if adaptive.ResponseTokenBudget > 0 {
			opts.ResponseTokenBudget = adaptive.ResponseTokenBudget
		}
		if adaptive.MaxIterations > 0 {
			opts.MaxIterations = adaptive.MaxIterations
		}
		if adaptive.EnableChunking {
			opts.EnableChunking = true
		}
		if adaptive.EscalateTier {
			tier.ToolCall = bumpUp(tier.ToolCall)
			if name := o.cfg.Tiers.Resolve(tier.ToolCall); name != "" {
				opts.ToolCallModel = name
			}
		}
	}

	log.L().Infof(
		"[moe] plan: expert=%s tier(tool=%s,synth=%s,reason=%s) skills=%v",
		e.ID, tier.ToolCall, tier.Synthesis, tier.Reasoning, e.SkillIDs,
	)

	aug := skillCtx
	if e.PromptAugmentation != "" {
		aug += "\n\n" + e.PromptAugmentation
	}

	return &Plan{
		Expert:                   e,
		Selection:                sel,
		SkillContext:             skillCtx,
		Tools:                    tools,
		ToolNames:                toolNames(tools),
		Options:                  opts,
		Tier:                     tier,
		Adaptive:                 adaptive,
		LoadedSkills:             loadedSkills,
		OmittedSkills:            omittedSkills,
		PolicyVersion:            policyVersion(admittedPolicy),
		CatalogVersion:           o.cfg.CatalogVersion,
		ContextDigest:            o.cfg.ContextDigest,
		ToolBindings:             bindings,
		SystemPromptAugmentation: aug,
		IsCentral:                isCentral,
	}, nil
}

func (o *Orchestrator) resolveTools(toolSet string) ([]llms.Tool, map[string]ToolBinding) {
	if toolSet == "" {
		return nil, nil
	}
	if o.cfg.CapabilityResolver != nil {
		resolved := o.cfg.CapabilityResolver(toolSet)
		tools := make([]llms.Tool, 0, len(resolved))
		bindings := make(map[string]ToolBinding, len(resolved))
		for _, binding := range resolved {
			if binding.Tool.Function == nil || binding.Tool.Function.Name == "" || binding.Ref.Name != binding.Tool.Function.Name || binding.Ref.Version == "" || binding.SchemaDigest == "" {
				continue
			}
			tools = append(tools, binding.Tool)
			bindings[binding.Ref.Name] = binding
		}
		return tools, bindings
	}
	if o.cfg.ToolResolver != nil {
		return o.cfg.ToolResolver(toolSet), nil
	}
	return nil, nil
}

func (o *Orchestrator) policySkillIDs(ids []string, snapshot *policy.Snapshot) []string {
	if snapshot == nil || o.cfg.Skills == nil {
		return append([]string(nil), ids...)
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		skill, ok := o.cfg.Skills.Get(id)
		if ok && snapshot.Decide("skill."+id, skill.Scope).Effect != policy.EffectDeny {
			out = append(out, id)
		}
	}
	return out
}

func filterBindings(bindings map[string]ToolBinding, snapshot policy.Snapshot) map[string]ToolBinding {
	out := make(map[string]ToolBinding, len(bindings))
	for name, binding := range bindings {
		if snapshot.Decide(name, "").Effect != policy.EffectDeny {
			out[name] = binding
		}
	}
	return out
}

func policyVersion(snapshot *policy.Snapshot) string {
	if snapshot == nil {
		return ""
	}
	return snapshot.Version()
}

func (o *Orchestrator) visibleExperts(snapshot *policy.Snapshot) []*ExpertDefinition {
	experts := o.cfg.Experts.All()
	if snapshot == nil {
		return experts
	}
	out := make([]*ExpertDefinition, 0, len(experts))
	for _, expert := range experts {
		if snapshot.Decide("expert."+string(expert.ID), "").Effect != policy.EffectDeny {
			copyExpert := *expert
			copyExpert.SkillIDs = o.policySkillIDs(expert.SkillIDs, snapshot)
			out = append(out, &copyExpert)
		}
	}
	return out
}
