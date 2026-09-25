package moe

import (
	"math"
	"sort"
	"strings"
	"unicode"
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
	// to the general expert (or the highest-priority non-synthesis expert).
	// Default: 0.15.
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
	patterns map[*ExpertDefinition]expertPatterns
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
	patterns := make(map[*ExpertDefinition]expertPatterns)
	for _, expert := range reg.All() {
		patterns[expert] = compileExpertPatterns(expert)
	}
	return &Router{registry: reg, cfg: cfg, patterns: patterns}
}

// Route picks an expert for the given query and context.
func (r *Router) Route(query string, ctx RoutingContext) *Selection {
	experts := r.registry.All()
	if len(experts) == 0 {
		return &Selection{Reasoning: "no experts registered"}
	}
	input := newLexicalInput(query)
	topics := make([]lexicalInput, len(ctx.Topics))
	for index, topic := range ctx.Topics {
		topics[index] = newLexicalInput(topic)
	}
	confidences := make(map[ExpertID]float64, len(experts))
	scores := make(map[ExpertID]expertScore, len(experts))
	var totalRaw float64
	for _, e := range experts {
		score := r.scoreExpertInput(e, input, topics, ctx)
		scores[e.ID] = score
		if score.raw > 0 {
			totalRaw += score.raw
		}
	}
	// Smooth + normalize to [0,1].
	for id, score := range scores {
		confidences[id] = smoothConfidence(score.raw, totalRaw)
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
	reasoning := explainScore(scores[primary.ID])

	// Cross-domain escalation requires at least two independent lexical signals
	// for each of two non-synthesis domains. Context alone cannot manufacture a
	// cross-domain request, and a synthesis keyword cannot act as the runner-up.
	if !primary.CanSynthesize {
		if runnerUp := credibleRunnerUp(experts, scores, primary, r.cfg.KeywordWeight*2); runnerUp != nil {
			ps, rs := scores[primary.ID], scores[runnerUp.ID]
			if ps.raw > 0 && rs.raw/ps.raw >= r.cfg.SynthesisThreshold {
				if synth, ok := r.registry.SynthesisExpert(); ok && synth.ID != primary.ID {
					return &Selection{
						Primary: synth, Secondary: []*ExpertDefinition{primary, runnerUp},
						Confidences: confidences,
						Reasoning: "Distinct domain evidence matched " + string(primary.ID) +
							" and " + string(runnerUp.ID) + "; escalating to synthesis expert.",
					}
				}
			}
		}
	}

	// Low-confidence fallback.
	if confidences[primary.ID] < r.cfg.MinConfidence {
		fallback := fallbackExpert(experts)
		if fallback != nil {
			return &Selection{Primary: fallback, Confidences: confidences, Reasoning: "No domain had credible evidence; using the general fallback."}
		}
	}

	// Secondaries must have positive raw evidence; sigmoid smoothing must not
	// turn a zero-evidence expert into an apparent peer.
	var secondaries []*ExpertDefinition
	primaryRaw := scores[primary.ID].raw
	for _, e := range experts[1:] {
		if primaryRaw > 0 && scores[e.ID].raw > 0 && scores[e.ID].raw/primaryRaw >= r.cfg.SecondaryThreshold {
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

type expertScore struct {
	raw       float64
	lexical   float64
	context   float64
	matched   []string
	penalties []string
}

type lexicalInput struct {
	tokens []string
}

type tokenPattern struct {
	raw    string
	tokens []string
}

type expertPatterns struct {
	positive []tokenPattern
	negative []tokenPattern
}

func newLexicalInput(value string) lexicalInput {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			current.WriteRune(char)
			continue
		}
		flush()
	}
	flush()
	return lexicalInput{tokens: tokens}
}

func (in lexicalInput) matches(pattern tokenPattern) bool {
	wanted := pattern.tokens
	if len(wanted) == 0 || len(wanted) > len(in.tokens) {
		return false
	}
	for start := 0; start <= len(in.tokens)-len(wanted); start++ {
		matched := true
		for offset := range wanted {
			if in.tokens[start+offset] != wanted[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func (r *Router) scoreExpert(e *ExpertDefinition, query string, ctx RoutingContext) float64 {
	topics := make([]lexicalInput, len(ctx.Topics))
	for index, topic := range ctx.Topics {
		topics[index] = newLexicalInput(topic)
	}
	return r.scoreExpertInput(e, newLexicalInput(query), topics, ctx).raw
}

func (r *Router) scoreExpertInput(e *ExpertDefinition, input lexicalInput, topics []lexicalInput, ctx RoutingContext) expertScore {
	var result expertScore
	patterns, ok := r.patterns[e]
	if !ok {
		patterns = compileExpertPatterns(e)
	}
	for _, keyword := range patterns.positive {
		if input.matches(keyword) {
			result.lexical += r.cfg.KeywordWeight
			result.matched = append(result.matched, keyword.raw)
		}
	}
	for _, keyword := range patterns.negative {
		if input.matches(keyword) {
			result.lexical -= r.cfg.NegativeWeight
			result.penalties = append(result.penalties, keyword.raw)
		}
	}
	// Independent context boosts let a genuinely cross-domain request surface
	// more than one qualified expert.
	if ctx.HasTraceContext && containsAny(e.Keywords, []string{"trace", "telemetry", "latency", "span"}) {
		result.context += r.cfg.ContextWeight
	}
	if ctx.HasBuildContext && containsAny(e.Keywords, []string{"build", "pipeline", "ci", "cd"}) {
		result.context += r.cfg.ContextWeight
	}
	if ctx.HasIssueContext && containsAny(e.Keywords, []string{"issue", "ticket", "bug"}) {
		result.context += r.cfg.ContextWeight
	}
	if ctx.HasCodeContext && containsAny(e.Keywords, []string{"code", "diff", "commit", "review"}) {
		result.context += r.cfg.ContextWeight
	}
	for _, topic := range topics {
		for _, keyword := range patterns.positive {
			if len(topic.tokens) == len(keyword.tokens) && topic.matches(keyword) {
				result.context += r.cfg.ContextWeight * 0.5
			}
		}
	}
	result.raw = result.lexical + result.context
	return result
}

func compileExpertPatterns(expert *ExpertDefinition) expertPatterns {
	patterns := expertPatterns{
		positive: make([]tokenPattern, 0, len(expert.Keywords)),
		negative: make([]tokenPattern, 0, len(expert.NegativeKeywords)),
	}
	for _, keyword := range expert.Keywords {
		patterns.positive = append(patterns.positive, tokenPattern{raw: keyword, tokens: newLexicalInput(keyword).tokens})
	}
	for _, keyword := range expert.NegativeKeywords {
		patterns.negative = append(patterns.negative, tokenPattern{raw: keyword, tokens: newLexicalInput(keyword).tokens})
	}
	return patterns
}

func explainScore(score expertScore) string {
	if len(score.matched) == 0 {
		return "Selected by context or deterministic fallback ranking."
	}
	reason := "Matched terms: " + strings.Join(score.matched, ", ")
	if len(score.penalties) > 0 {
		reason += "; negative terms: " + strings.Join(score.penalties, ", ")
	}
	return reason
}

func credibleRunnerUp(experts []*ExpertDefinition, scores map[ExpertID]expertScore, primary *ExpertDefinition, minEvidence float64) *ExpertDefinition {
	if primary.ID == ExpertID("general") || scores[primary.ID].lexical < minEvidence {
		return nil
	}
	for _, expert := range experts {
		if expert.ID != primary.ID && expert.ID != ExpertID("general") && !expert.CanSynthesize && scores[expert.ID].lexical >= minEvidence {
			return expert
		}
	}
	return nil
}

func fallbackExpert(experts []*ExpertDefinition) *ExpertDefinition {
	for _, expert := range experts {
		if expert.ID == ExpertID("general") {
			return expert
		}
	}
	for _, expert := range experts {
		if !expert.CanSynthesize {
			return expert
		}
	}
	return experts[0]
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
