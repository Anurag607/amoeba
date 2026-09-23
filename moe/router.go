package moe

import (
	"math"
	"sort"
	"strings"
)

// RoutingContext is the optional ambient context the router can use to
// boost expert scores when explicit query keywords are sparse. All fields
// are optional.
type RoutingContext struct {
	// Topics is a free-form set of tags about the active session
	// (e.g. service names, repo names, "trace", "build", "issue").
	Topics []string

	// HasTraceContext: the user is looking at telemetry / a trace.
	HasTraceContext bool

	// HasBuildContext: the user is looking at a CI build / pipeline.
	HasBuildContext bool

	// HasIssueContext: the user is looking at an issue / ticket.
	HasIssueContext bool

	// HasCodeContext: the user is looking at code / a diff.
	HasCodeContext bool

	// HistoryDepth is the count of prior turns in the conversation.
	HistoryDepth int
}

// RouterConfig tunes the router's behavior.
type RouterConfig struct {
	// SynthesisThreshold: when the second-best expert score is at least this
	// fraction of the best, the router escalates to the synthesis expert.
	// Default: 0.6.
	SynthesisThreshold float64

	// MinConfidence: if the best score is below this, the router falls back
	// to the synthesis expert (or the first registered expert). Default: 0.15.
	MinConfidence float64

	// SecondaryThreshold: secondaries reported on Selection must score at
	// least this fraction of the primary's score. Default: 0.5.
	SecondaryThreshold float64

	// KeywordWeight, NegativeWeight, ContextWeight are the per-source
	// score weights. Defaults: 1.0, 1.5, 0.4.
	KeywordWeight  float64
	NegativeWeight float64
	ContextWeight  float64
}

// DefaultRouterConfig returns sensible defaults.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		SynthesisThreshold: 0.6,
		MinConfidence:      0.15,
		SecondaryThreshold: 0.5,
		KeywordWeight:      1.0,
		NegativeWeight:     1.5,
		ContextWeight:      0.4,
	}
}

// Router scores experts against a query + routing context.
type Router struct {
	registry *Registry
	cfg      RouterConfig
}

// NewRouter constructs a Router. Pass DefaultRouterConfig() for sensible
// defaults.
func NewRouter(reg *Registry, cfg RouterConfig) *Router {
	defaults := DefaultRouterConfig()
	if cfg.SynthesisThreshold == 0 {
		cfg.SynthesisThreshold = defaults.SynthesisThreshold
	}
	if cfg.MinConfidence == 0 {
		cfg.MinConfidence = defaults.MinConfidence
	}
	if cfg.SecondaryThreshold == 0 {
		cfg.SecondaryThreshold = defaults.SecondaryThreshold
	}
	if cfg.KeywordWeight == 0 {
		cfg.KeywordWeight = defaults.KeywordWeight
	}
	if cfg.NegativeWeight == 0 {
		cfg.NegativeWeight = defaults.NegativeWeight
	}
	if cfg.ContextWeight == 0 {
		cfg.ContextWeight = defaults.ContextWeight
	}
	return &Router{registry: reg, cfg: cfg}
}

// Route picks an expert for the given query and context.
func (r *Router) Route(query string, ctx RoutingContext) *Selection {
	experts := r.registry.All()
	if len(experts) == 0 {
		return &Selection{Reasoning: "no experts registered"}
	}
	lowQuery := strings.ToLower(query)
	confidences := make(map[ExpertID]float64, len(experts))
	rawScores := make(map[ExpertID]float64, len(experts))
	var totalRaw float64
	for _, e := range experts {
		s := r.scoreExpert(e, lowQuery, ctx)
		rawScores[e.ID] = s
		if s > 0 {
			totalRaw += s
		}
	}
	// Smooth + normalize to [0,1].
	for id, s := range rawScores {
		confidences[id] = smoothConfidence(s, totalRaw)
	}

	// Sort experts by confidence descending; priority breaks ties.
	sort.SliceStable(experts, func(i, j int) bool {
		ci, cj := confidences[experts[i].ID], confidences[experts[j].ID]
		if ci != cj {
			return ci > cj
		}
		return experts[i].Priority < experts[j].Priority
	})

	primary := experts[0]
	reasoning := r.explainScore(primary, lowQuery, ctx)

	// Cross-domain synthesis escalation.
	if !primary.CanSynthesize && len(experts) > 1 {
		runnerUp := experts[1]
		pc := confidences[primary.ID]
		rc := confidences[runnerUp.ID]
		if pc > 0 && rc/pc >= r.cfg.SynthesisThreshold {
			if synth, ok := r.registry.SynthesisExpert(); ok && synth.ID != primary.ID {
				return &Selection{
					Primary:     synth,
					Secondary:   []*ExpertDefinition{primary, runnerUp},
					Confidences: confidences,
					Reasoning: "Multiple domains scored highly (" +
						string(primary.ID) + " and " + string(runnerUp.ID) +
						") — escalating to synthesis expert.",
				}
			}
		}
	}

	// Low-confidence fallback.
	if confidences[primary.ID] < r.cfg.MinConfidence {
		if synth, ok := r.registry.SynthesisExpert(); ok {
			return &Selection{
				Primary:     synth,
				Confidences: confidences,
				Reasoning:   "No expert scored confidently — falling back to synthesis expert.",
			}
		}
	}

	// Collect secondaries above SecondaryThreshold * primary confidence.
	var secondaries []*ExpertDefinition
	pc := confidences[primary.ID]
	for _, e := range experts[1:] {
		if pc > 0 && confidences[e.ID]/pc >= r.cfg.SecondaryThreshold {
			secondaries = append(secondaries, e)
		}
	}

	return &Selection{
		Primary:     primary,
		Secondary:   secondaries,
		Confidences: confidences,
		Reasoning:   reasoning,
	}
}

func (r *Router) scoreExpert(e *ExpertDefinition, lowQuery string, ctx RoutingContext) float64 {
	var score float64
	for _, kw := range e.Keywords {
		if strings.Contains(lowQuery, strings.ToLower(kw)) {
			score += r.cfg.KeywordWeight
		}
	}
	for _, kw := range e.NegativeKeywords {
		if strings.Contains(lowQuery, strings.ToLower(kw)) {
			score -= r.cfg.NegativeWeight
		}
	}
	// Independent context boosts let a genuinely cross-domain request surface
	// more than one qualified expert.
	if ctx.HasTraceContext && containsAny(e.Keywords, []string{"trace", "telemetry", "latency", "span"}) {
		score += r.cfg.ContextWeight
	}
	if ctx.HasBuildContext && containsAny(e.Keywords, []string{"build", "pipeline", "ci", "cd"}) {
		score += r.cfg.ContextWeight
	}
	if ctx.HasIssueContext && containsAny(e.Keywords, []string{"issue", "ticket", "bug"}) {
		score += r.cfg.ContextWeight
	}
	if ctx.HasCodeContext && containsAny(e.Keywords, []string{"code", "diff", "commit", "review"}) {
		score += r.cfg.ContextWeight
	}
	for _, tag := range ctx.Topics {
		for _, kw := range e.Keywords {
			if strings.EqualFold(tag, kw) {
				score += r.cfg.ContextWeight * 0.5
			}
		}
	}
	return score
}

func (r *Router) explainScore(e *ExpertDefinition, lowQuery string, _ RoutingContext) string {
	var matched []string
	for _, kw := range e.Keywords {
		if strings.Contains(lowQuery, strings.ToLower(kw)) {
			matched = append(matched, kw)
		}
	}
	if len(matched) == 0 {
		return "Selected by context / default ranking."
	}
	return "Matched keywords: " + strings.Join(matched, ", ")
}

// smoothConfidence maps a raw score in (-∞, +∞) to a confidence in [0, 1]
// via a sigmoid centered at total/N, so scores get spread across the
// available range rather than clipping.
func smoothConfidence(score, total float64) float64 {
	if score <= 0 {
		return 0
	}
	if total <= 0 {
		return 0
	}
	// 1/(1 + e^{-k*(score - total*0.25)})
	const k = 1.5
	x := score - total*0.25
	return 1.0 / (1.0 + math.Exp(-k*x))
}

func containsAny(set, needles []string) bool {
	for _, s := range set {
		for _, n := range needles {
			if strings.EqualFold(s, n) {
				return true
			}
		}
	}
	return false
}
