package moe

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type StatsKey struct{ Provider, Model, TaskType string }
type RunStats struct {
	Samples              int64
	SuccessRate          float64
	AvgInputTokens       float64
	AvgOutputTokens      float64
	AvgLatency           time.Duration
	AvgToolFailures      float64
	AvgChunks            float64
	AvgPeakUtilization   float64
	ContinuationRate     float64
	PreflightSuccessRate float64
	AvgCostUSD           float64
	AvgQualityScore      float64
	ColdStartRate        float64
}
type RunSample struct {
	Key             StatsKey
	InputTokens     int
	OutputTokens    int
	Success         bool
	Latency         time.Duration
	ToolFailures    int
	Chunks          int
	PeakUtilization float64
	Continued       bool
	PreflightOK     bool
	CostUSD         float64
	QualityScore    float64
	ColdStart       bool
}

type StatsLookup interface {
	Lookup(StatsKey) (RunStats, bool)
}
type StatsRecorder interface{ Record(RunSample) error }

type LearningConfig struct {
	Disabled bool
	Lookup   StatsLookup
	Recorder StatsRecorder
}

type AdaptiveInput struct {
	Key                  StatsKey
	EstimatedInputTokens int
	EffectiveContext     int
	HasTools             bool
	Base                 LoopOptions
}

type AdaptiveDecision struct {
	ResponseTokenBudget int    `json:"response_token_budget,omitempty"`
	MaxIterations       int    `json:"max_iterations,omitempty"`
	EnableChunking      bool   `json:"enable_chunking,omitempty"`
	EscalateTier        bool   `json:"escalate_tier,omitempty"`
	Reason              string `json:"reason,omitempty"`
}

type AdaptivePlanner interface {
	Recommend(AdaptiveInput, StatsLookup) AdaptiveDecision
}

type DefaultAdaptivePlanner struct {
	MinSamples  int64
	SafetyRatio float64
	MinResponse int
}

func (p DefaultAdaptivePlanner) Recommend(input AdaptiveInput, lookup StatsLookup) AdaptiveDecision {
	minSamples := p.MinSamples
	if minSamples <= 0 {
		minSamples = 5
	}
	safety := p.SafetyRatio
	if safety <= 0 || safety >= 1 {
		safety = 0.9
	}
	minResponse := p.MinResponse
	if minResponse <= 0 {
		minResponse = 512
	}
	decision := AdaptiveDecision{ResponseTokenBudget: input.Base.ResponseTokenBudget, MaxIterations: input.Base.MaxIterations, Reason: "baseline"}
	stats, ok := RunStats{}, false
	if lookup != nil {
		stats, ok = lookup.Lookup(input.Key)
	}
	if ok && stats.Samples >= minSamples {
		decision.ResponseTokenBudget = int(math.Ceil(stats.AvgOutputTokens * 1.25))
		if decision.ResponseTokenBudget < minResponse {
			decision.ResponseTokenBudget = minResponse
		}
		if stats.ContinuationRate > 0.25 {
			decision.EscalateTier = true
		}
		decision.Reason = fmt.Sprintf("learned from %d samples", stats.Samples)
	}
	if decision.ResponseTokenBudget <= 0 {
		decision.ResponseTokenBudget = minResponse
	}
	if input.EffectiveContext > 0 {
		usable := int(float64(input.EffectiveContext) * safety)
		if input.EstimatedInputTokens+decision.ResponseTokenBudget > usable {
			if !input.HasTools {
				decision.EnableChunking = true
			}
			available := usable - input.EstimatedInputTokens
			if available > 0 && available < decision.ResponseTokenBudget {
				decision.ResponseTokenBudget = available
			}
			decision.Reason += "; context fit"
		}
	}
	return decision
}

// EWMAStore is a concurrency-safe reference learning store.
type EWMAStore struct {
	mu    sync.RWMutex
	alpha float64
	stats map[StatsKey]RunStats
}

func NewEWMAStore(alpha float64) *EWMAStore {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.25
	}
	return &EWMAStore{alpha: alpha, stats: make(map[StatsKey]RunStats)}
}

func (s *EWMAStore) Lookup(key StatsKey) (RunStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats, ok := s.stats[key]
	return stats, ok
}

func (s *EWMAStore) Record(sample RunSample) error {
	if sample.Key.Model == "" || sample.Key.TaskType == "" || sample.InputTokens < 0 || sample.OutputTokens < 0 || sample.CostUSD < 0 || sample.QualityScore < 0 || sample.QualityScore > 1 {
		return fmt.Errorf("learning sample requires model, task type, and non-negative tokens")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.stats[sample.Key]
	a := s.alpha
	if stats.Samples == 0 {
		a = 1
	}
	stats.SuccessRate = ewma(stats.SuccessRate, boolFloat(sample.Success), a)
	stats.AvgInputTokens = ewma(stats.AvgInputTokens, float64(sample.InputTokens), a)
	stats.AvgOutputTokens = ewma(stats.AvgOutputTokens, float64(sample.OutputTokens), a)
	stats.AvgLatency = time.Duration(ewma(float64(stats.AvgLatency), float64(sample.Latency), a))
	stats.AvgToolFailures = ewma(stats.AvgToolFailures, float64(sample.ToolFailures), a)
	stats.AvgChunks = ewma(stats.AvgChunks, float64(sample.Chunks), a)
	stats.AvgPeakUtilization = ewma(stats.AvgPeakUtilization, sample.PeakUtilization, a)
	stats.ContinuationRate = ewma(stats.ContinuationRate, boolFloat(sample.Continued), a)
	stats.PreflightSuccessRate = ewma(stats.PreflightSuccessRate, boolFloat(sample.PreflightOK), a)
	stats.AvgCostUSD = ewma(stats.AvgCostUSD, sample.CostUSD, a)
	stats.AvgQualityScore = ewma(stats.AvgQualityScore, sample.QualityScore, a)
	stats.ColdStartRate = ewma(stats.ColdStartRate, boolFloat(sample.ColdStart), a)
	stats.Samples++
	s.stats[sample.Key] = stats
	return nil
}

func ewma(previous, sample, alpha float64) float64 { return alpha*sample + (1-alpha)*previous }
func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func (o *Orchestrator) RecordSample(sample RunSample) error {
	if o.cfg.Learning.Disabled || o.cfg.Learning.Recorder == nil {
		return nil
	}
	return o.cfg.Learning.Recorder.Record(sample)
}

// RecordOutcome records both the selected expert and the MoE strategy from
// one host-reported outcome, avoiding missing strategy-level feedback.
func (o *Orchestrator) RecordOutcome(plan *Plan, sample RunSample) error {
	if plan == nil || plan.Expert == nil {
		return fmt.Errorf("record outcome requires an expert plan")
	}
	sample.Key.TaskType = string(plan.Expert.ID)
	if err := o.RecordSample(sample); err != nil {
		return err
	}
	sample.Key.Provider = "moe-engine"
	sample.Key.Model = plan.Tier.ToolCall.String() + ":" + plan.Tier.Synthesis.String()
	sample.Key.TaskType = string(plan.Expert.ID)
	return o.RecordSample(sample)
}
