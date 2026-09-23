package moe

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// RoutingObjective weights measured quality against operational cost. Zero
// values receive conservative defaults so selection remains deterministic.
type RoutingObjective struct {
	MinSamples        int64
	RequiredContext   int
	RequireTools      bool
	RequireStructured bool
	QualityWeight     float64
	LatencyWeight     float64
	CostWeight        float64
	ReliabilityWeight float64
	ColdStartWeight   float64
	UncertaintyWeight float64
	LatencyScale      time.Duration
	CostScaleUSD      float64
	ColdStartScale    time.Duration
}

type ProviderScore struct {
	Candidate ProviderCandidate `json:"candidate"`
	Score     float64           `json:"score"`
	Eligible  bool              `json:"eligible"`
	Reason    string            `json:"reason"`
	Samples   int64             `json:"samples"`
}

// RankProviders returns every candidate, eligible candidates first and then
// by descending score. Unknown providers receive a prior plus an uncertainty
// penalty rather than being mistaken for proven high-quality routes.
func RankProviders(candidates []ProviderCandidate, taskType string, lookup StatsLookup, objective RoutingObjective, now time.Time) ([]ProviderScore, error) {
	if taskType == "" {
		return nil, fmt.Errorf("provider ranking requires task type")
	}
	objective = defaultRoutingObjective(objective)
	out := make([]ProviderScore, 0, len(candidates))
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return nil, err
		}
		ranked := ProviderScore{Candidate: candidate, Eligible: true, Reason: "eligible"}
		switch {
		case !candidate.Ready:
			ranked.Eligible, ranked.Reason = false, "provider_not_ready"
		case !candidate.Health.ExpiresAt.IsZero() && !now.Before(candidate.Health.ExpiresAt):
			ranked.Eligible, ranked.Reason = false, "provider_health_stale"
		case candidate.ContextWindow < objective.RequiredContext:
			ranked.Eligible, ranked.Reason = false, "context_too_small"
		case objective.RequireTools && !candidate.ToolCalling:
			ranked.Eligible, ranked.Reason = false, "tool_calling_unavailable"
		case objective.RequireStructured && !candidate.StructuredOutput:
			ranked.Eligible, ranked.Reason = false, "structured_output_unavailable"
		}
		stats, found := RunStats{}, false
		if lookup != nil {
			stats, found = lookup.Lookup(StatsKey{Provider: candidate.Provider, Model: candidate.Model, TaskType: taskType})
		}
		ranked.Samples = stats.Samples
		quality, reliability := 0.5, 0.5
		if found && stats.Samples >= objective.MinSamples {
			quality = stats.AvgQualityScore
			if quality == 0 {
				quality = stats.SuccessRate
			}
			reliability = stats.SuccessRate * stats.PreflightSuccessRate
		}
		latency := float64(stats.AvgLatency) / float64(objective.LatencyScale)
		cost := stats.AvgCostUSD / objective.CostScaleUSD
		coldStart := float64(candidate.Resources.ColdStart) / float64(objective.ColdStartScale)
		uncertainty := 1 / math.Sqrt(float64(stats.Samples)+1)
		ranked.Score = objective.QualityWeight*quality + objective.ReliabilityWeight*reliability -
			objective.LatencyWeight*latency - objective.CostWeight*cost -
			objective.ColdStartWeight*coldStart - objective.UncertaintyWeight*uncertainty
		out = append(out, ranked)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Eligible != out[j].Eligible {
			return out[i].Eligible
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		left := out[i].Candidate.Provider + "\x00" + out[i].Candidate.Model
		right := out[j].Candidate.Provider + "\x00" + out[j].Candidate.Model
		return left < right
	})
	return out, nil
}

func defaultRoutingObjective(in RoutingObjective) RoutingObjective {
	if in.MinSamples <= 0 {
		in.MinSamples = 5
	}
	if in.QualityWeight == 0 && in.LatencyWeight == 0 && in.CostWeight == 0 && in.ReliabilityWeight == 0 && in.ColdStartWeight == 0 && in.UncertaintyWeight == 0 {
		in.QualityWeight, in.ReliabilityWeight = 0.45, 0.35
		in.LatencyWeight, in.CostWeight = 0.08, 0.06
		in.ColdStartWeight, in.UncertaintyWeight = 0.03, 0.03
	}
	if in.LatencyScale <= 0 {
		in.LatencyScale = 10 * time.Second
	}
	if in.CostScaleUSD <= 0 {
		in.CostScaleUSD = 1
	}
	if in.ColdStartScale <= 0 {
		in.ColdStartScale = 10 * time.Second
	}
	return in
}
