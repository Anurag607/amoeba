package runtimekit

import (
	"time"

	"github.com/anurgosw/agentic-moe/moe"
)

// RoutingInput is the transport-safe routing context.
type RoutingInput struct {
	Topics          []string `json:"topics,omitempty"`
	HasTraceContext bool     `json:"has_trace_context,omitempty"`
	HasBuildContext bool     `json:"has_build_context,omitempty"`
	HasIssueContext bool     `json:"has_issue_context,omitempty"`
	HasCodeContext  bool     `json:"has_code_context,omitempty"`
	HistoryDepth    int      `json:"history_depth,omitempty"`
}

func (r RoutingInput) toMOE() moe.RoutingContext {
	return moe.RoutingContext{Topics: append([]string(nil), r.Topics...), HasTraceContext: r.HasTraceContext, HasBuildContext: r.HasBuildContext, HasIssueContext: r.HasIssueContext, HasCodeContext: r.HasCodeContext, HistoryDepth: r.HistoryDepth}
}

// ExpertView is a stable public projection of an expert definition.
type ExpertView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	DefaultTier   string `json:"default_tier"`
	CanSynthesize bool   `json:"can_synthesize,omitempty"`
}

// SelectionView describes a routing decision without leaking mutable engine pointers.
type SelectionView struct {
	Primary     string             `json:"primary"`
	Secondary   []string           `json:"secondary,omitempty"`
	Confidences map[string]float64 `json:"confidences"`
	Reasoning   string             `json:"reasoning"`
}

// PlanOptions are execution limits for the host-owned model loop.
type PlanOptions struct {
	ToolCallModel       string `json:"tool_call_model,omitempty"`
	SynthesisModel      string `json:"synthesis_model,omitempty"`
	ResponseTokenBudget int    `json:"response_token_budget"`
	EnableChunking      bool   `json:"enable_chunking"`
	MaxIterations       int    `json:"max_iterations"`
	MaxToolCalls        int    `json:"max_tool_calls"`
}

// TierView contains the abstract model-tier decision.
type TierView struct {
	ToolCall  string `json:"tool_call"`
	Synthesis string `json:"synthesis"`
	Reasoning string `json:"reasoning"`
}

// PlanView is the stable, serializable planner result used by all adapters.
type PlanView struct {
	Expert                   ExpertView    `json:"expert"`
	Selection                SelectionView `json:"selection"`
	SystemPromptAugmentation string        `json:"system_prompt_augmentation,omitempty"`
	ToolNames                []string      `json:"tool_names,omitempty"`
	LoadedSkills             []string      `json:"loaded_skills,omitempty"`
	OmittedSkills            []string      `json:"omitted_skills,omitempty"`
	Options                  PlanOptions   `json:"options"`
	Tier                     TierView      `json:"tier"`
	PolicyVersion            string        `json:"policy_version,omitempty"`
	CatalogVersion           string        `json:"catalog_version,omitempty"`
	ContextDigest            string        `json:"context_digest,omitempty"`
}

// Manifest describes capabilities without initiating provider access.
type Manifest struct {
	SchemaVersion      int          `json:"schema_version"`
	Experts            []ExpertView `json:"experts"`
	Providers          []string     `json:"providers,omitempty"`
	SideEffectsEnabled bool         `json:"side_effects_enabled"`
	Transports         []string     `json:"transports"`
}

// ProviderStatus is one provider's health result.
type ProviderStatus struct {
	Name           string `json:"name"`
	Ready          bool   `json:"ready"`
	CandidateCount int    `json:"candidate_count"`
	Error          string `json:"error,omitempty"`
}

// HealthReport describes runtime and provider readiness.
type HealthReport struct {
	Status    string           `json:"status"`
	CheckedAt time.Time        `json:"checked_at"`
	Providers []ProviderStatus `json:"providers"`
}

func makePlanView(p *moe.Plan) PlanView {
	view := PlanView{
		SystemPromptAugmentation: p.SystemPromptAugmentation, ToolNames: append([]string(nil), p.ToolNames...), LoadedSkills: append([]string(nil), p.LoadedSkills...), OmittedSkills: append([]string(nil), p.OmittedSkills...),
		Options: PlanOptions{ToolCallModel: p.Options.ToolCallModel, SynthesisModel: p.Options.SynthesisModel, ResponseTokenBudget: p.Options.ResponseTokenBudget, EnableChunking: p.Options.EnableChunking, MaxIterations: p.Options.MaxIterations, MaxToolCalls: p.Options.MaxToolCalls},
		Tier:    TierView{ToolCall: p.Tier.ToolCall.String(), Synthesis: p.Tier.Synthesis.String(), Reasoning: p.Tier.Reasoning}, PolicyVersion: p.PolicyVersion, CatalogVersion: p.CatalogVersion, ContextDigest: p.ContextDigest,
	}
	if p.Expert != nil {
		view.Expert = ExpertView{ID: string(p.Expert.ID), Name: p.Expert.Name, Description: p.Expert.Description, DefaultTier: p.Expert.DefaultTier.String(), CanSynthesize: p.Expert.CanSynthesize}
	}
	if p.Selection != nil {
		view.Selection = SelectionView{Reasoning: p.Selection.Reasoning, Confidences: make(map[string]float64, len(p.Selection.Confidences))}
		if p.Selection.Primary != nil {
			view.Selection.Primary = string(p.Selection.Primary.ID)
		}
		for _, e := range p.Selection.Secondary {
			view.Selection.Secondary = append(view.Selection.Secondary, string(e.ID))
		}
		for id, confidence := range p.Selection.Confidences {
			view.Selection.Confidences[string(id)] = confidence
		}
	}
	return view
}
