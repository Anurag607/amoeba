// Package moe implements the Mixture-of-Experts orchestration primitives:
// an expert registry, a keyword+context router, a model-tier classifier, a
// planner-style orchestrator, and the central / delegate cross-expert tools.
//
// The framework is consumer-driven: a project supplies its own expert
// definitions (with associated skill IDs, tool-set names, and prompts), a
// skills.Registry, and a tool resolver. The orchestrator returns a Plan
// the caller executes by running its own agentic loop.
package moe

import (
	"fmt"
	"strings"
	"sync"

	"github.com/anurgosw/agentic-moe/log"
	"github.com/anurgosw/agentic-moe/skills"
)

// ExpertID uniquely identifies an expert.
type ExpertID string

// ExpertDefinition describes one expert's behavior. Consumers register
// definitions at startup; the router scores queries against keywords,
// negative keywords, and routing context.
type ExpertDefinition struct {
	ID          ExpertID
	Name        string
	Description string

	// SkillIDs the orchestrator will pull from the skills registry and inject
	// into the system prompt for this expert.
	SkillIDs []string

	// ToolSetName the consumer's tool resolver maps to a []llms.Tool.
	ToolSetName string

	// Keywords contribute positive routing score on overlap with the query.
	Keywords []string

	// NegativeKeywords contribute negative routing score.
	NegativeKeywords []string

	// Priority is a tie-breaker (lower = preferred) when scores are equal.
	Priority int

	// CanSynthesize: true for experts that handle cross-domain synthesis.
	// When a non-synthesis expert wins routing but a second expert is also
	// confident, the router escalates to the synthesis expert if one exists.
	CanSynthesize bool

	// DefaultTier is the model tier this expert prefers when no
	// per-request override is supplied. ModelTierBalanced when zero.
	DefaultTier ModelTier

	// Loop overrides — zero values mean "use the caller's defaults".
	MaxIterations            int
	MaxToolCalls             int
	MinToolCallsBeforeFinish int

	// PromptAugmentation is appended to the system prompt after the
	// skill context.
	PromptAugmentation string
}

// AssembleSkillContext renders this expert's skill bodies via the registry.
func (e *ExpertDefinition) AssembleSkillContext(reg *skills.Registry) string {
	if reg == nil {
		return ""
	}
	return reg.AssembleContext(e.SkillIDs)
}

// Registry holds expert definitions.
type Registry struct {
	mu      sync.RWMutex
	experts map[ExpertID]*ExpertDefinition
	order   []ExpertID
}

// NewRegistry constructs an empty expert registry.
func NewRegistry() *Registry {
	return &Registry{experts: make(map[ExpertID]*ExpertDefinition)}
}

// Register adds (or replaces) an expert.
func (r *Registry) Register(e *ExpertDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.experts[e.ID]; !exists {
		r.order = append(r.order, e.ID)
	}
	r.experts[e.ID] = e
	log.L().Infof("[moe] registered expert: %s (%s)", e.ID, e.Name)
}

// Get fetches an expert by ID.
func (r *Registry) Get(id ExpertID) (*ExpertDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.experts[id]
	return e, ok
}

// All returns every registered expert, in registration order.
func (r *Registry) All() []*ExpertDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ExpertDefinition, 0, len(r.experts))
	for _, id := range r.order {
		out = append(out, r.experts[id])
	}
	return out
}

// IDs returns the IDs of all registered experts.
func (r *Registry) IDs() []ExpertID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ExpertID, 0, len(r.experts))
	out = append(out, r.order...)
	return out
}

// SynthesisExpert returns the first registered expert with CanSynthesize.
func (r *Registry) SynthesisExpert() (*ExpertDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range r.order {
		if r.experts[id].CanSynthesize {
			return r.experts[id], true
		}
	}
	return nil, false
}

// Selection is a router decision: a primary expert, zero or more
// secondaries, per-expert confidence scores, and a short reasoning trace.
type Selection struct {
	Primary     *ExpertDefinition
	Secondary   []*ExpertDefinition
	Confidences map[ExpertID]float64
	Reasoning   string
}

// String renders the selection compactly.
func (s *Selection) String() string {
	if s == nil || s.Primary == nil {
		return "(no selection)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Primary: %s (%.0f%%)", s.Primary.ID, s.Confidences[s.Primary.ID]*100)
	if len(s.Secondary) > 0 {
		names := make([]string, len(s.Secondary))
		for i, sec := range s.Secondary {
			names[i] = fmt.Sprintf("%s(%.0f%%)", sec.ID, s.Confidences[sec.ID]*100)
		}
		fmt.Fprintf(&sb, " | Secondary: %s", strings.Join(names, ", "))
	}
	return sb.String()
}
