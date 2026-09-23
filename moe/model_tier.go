package moe

import "strings"

// ModelTier is a coarse complexity bucket the orchestrator hands the
// caller so they can pick a concrete model name from their own catalog.
//
// Consumers map each tier to one or more model identifiers via TierMapping.
type ModelTier int

const (
	// ModelTierUnspecified means "no preference — caller uses its default".
	ModelTierUnspecified ModelTier = iota
	// ModelTierFast is for cheap, latency-sensitive steps (routing,
	// keyword extraction, short tool-arg generation).
	ModelTierFast
	// ModelTierBalanced is the default for most tool-calling turns.
	ModelTierBalanced
	// ModelTierStrong is reserved for multi-domain synthesis, long-context
	// reasoning, and complex planning.
	ModelTierStrong
)

// String renders the tier as a stable label (used in events / logs).
func (t ModelTier) String() string {
	switch t {
	case ModelTierFast:
		return "fast"
	case ModelTierBalanced:
		return "balanced"
	case ModelTierStrong:
		return "strong"
	default:
		return "unspecified"
	}
}

// TierMapping resolves a tier to a concrete model identifier. The empty
// string means "no override". Consumers typically build this once at
// startup from their config.
type TierMapping map[ModelTier]string

// Resolve returns the model name for `tier`, falling back to the
// Balanced entry, then to "".
func (m TierMapping) Resolve(tier ModelTier) string {
	if name, ok := m[tier]; ok && name != "" {
		return name
	}
	if name, ok := m[ModelTierBalanced]; ok && name != "" {
		return name
	}
	return ""
}

// ComplexityClassifier picks a tier per step (tool-calling vs synthesis)
// based on cheap, deterministic heuristics. It is intentionally simple —
// consumers can replace it with their own classifier by writing a function
// with the same signature and ignoring this struct entirely.
type ComplexityClassifier struct {
	// LongQueryRunes: queries longer than this push the tool-call step up
	// one tier. Default: 600.
	LongQueryRunes int

	// DeepHistoryTurns: when history depth exceeds this, push tool-call up
	// one tier. Default: 8.
	DeepHistoryTurns int

	// MultiDomainSecondaryConfidence: when secondaries exist above this
	// confidence, push synthesis up one tier. Default: 0.4.
	MultiDomainSecondaryConfidence float64
}

// DefaultClassifier returns a ComplexityClassifier with sensible defaults.
func DefaultClassifier() ComplexityClassifier {
	return ComplexityClassifier{
		LongQueryRunes:                 600,
		DeepHistoryTurns:               8,
		MultiDomainSecondaryConfidence: 0.4,
	}
}

// TierDecision is the per-step tier assignment for one planning request.
type TierDecision struct {
	ToolCall  ModelTier
	Synthesis ModelTier
	Reasoning string
}

// Classify produces a TierDecision given the routing inputs and selection.
// expertDefault is the expert's DefaultTier (may be ModelTierUnspecified).
func (c ComplexityClassifier) Classify(
	query string,
	ctx RoutingContext,
	sel *Selection,
	expertDefault ModelTier,
) TierDecision {
	base := expertDefault
	if base == ModelTierUnspecified {
		base = ModelTierBalanced
	}
	tool := base
	synth := base

	long := c.LongQueryRunes
	if long == 0 {
		long = 600
	}
	deep := c.DeepHistoryTurns
	if deep == 0 {
		deep = 8
	}
	threshold := c.MultiDomainSecondaryConfidence
	if threshold == 0 {
		threshold = 0.4
	}

	var reasons []string

	if len([]rune(query)) > long {
		tool = bumpUp(tool)
		reasons = append(reasons, "long query")
	}
	if ctx.HistoryDepth > deep {
		tool = bumpUp(tool)
		reasons = append(reasons, "deep history")
	}
	if sel != nil && sel.Primary != nil && sel.Primary.CanSynthesize {
		synth = bumpUp(synth)
		reasons = append(reasons, "synthesis expert")
	}
	if sel != nil {
		var strongSecondaries int
		for _, sec := range sel.Secondary {
			if sel.Confidences[sec.ID] >= threshold {
				strongSecondaries++
			}
		}
		if strongSecondaries > 0 {
			synth = bumpUp(synth)
			reasons = append(reasons, "multi-domain")
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "baseline")
	}
	return TierDecision{
		ToolCall:  tool,
		Synthesis: synth,
		Reasoning: strings.Join(reasons, ", "),
	}
}

func bumpUp(t ModelTier) ModelTier {
	switch t {
	case ModelTierUnspecified, ModelTierFast:
		return ModelTierBalanced
	case ModelTierBalanced:
		return ModelTierStrong
	default:
		return ModelTierStrong
	}
}
