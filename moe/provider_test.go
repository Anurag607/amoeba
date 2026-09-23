package moe

import (
	"testing"
	"time"
)

func TestProviderCandidateRequiresExplicitCapacityMetadata(t *testing.T) {
	valid := ProviderCandidate{Provider: "local", Model: "small", Tier: ModelTierFast, ContextWindow: 8192, MaxParallel: 2, Placement: "device", Ready: true}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.MaxParallel = 0
	if err := valid.Validate(); err == nil {
		t.Fatal("zero provider capacity accepted")
	}
}

func TestRankProvidersFiltersStaleAndUsesMeasuredQuality(t *testing.T) {
	store := NewEWMAStore(1)
	for _, sample := range []RunSample{
		{Key: StatsKey{Provider: "p", Model: "good", TaskType: "code"}, Success: true, PreflightOK: true, QualityScore: .9},
		{Key: StatsKey{Provider: "p", Model: "bad", TaskType: "code"}, Success: true, PreflightOK: true, QualityScore: .2},
	} {
		if err := store.Record(sample); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	base := ProviderCandidate{Provider: "p", Tier: ModelTierFast, ContextWindow: 8192, MaxParallel: 1, Placement: "local", Ready: true, ToolCalling: true}
	good, bad, stale := base, base, base
	good.Model, bad.Model, stale.Model = "good", "bad", "stale"
	stale.Health = ProviderHealth{ObservedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute)}
	ranked, err := RankProviders([]ProviderCandidate{bad, stale, good}, "code", store, RoutingObjective{MinSamples: 1, RequireTools: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if ranked[0].Candidate.Model != "good" || ranked[2].Eligible || ranked[2].Reason != "provider_health_stale" {
		t.Fatalf("ranked=%+v", ranked)
	}
}
