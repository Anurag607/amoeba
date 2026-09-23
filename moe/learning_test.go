package moe

import (
	"testing"
	"time"
)

func TestDefaultAdaptivePlannerUsesMinimumSamplesContextFitAndEscalation(t *testing.T) {
	store := NewEWMAStore(0.25)
	key := StatsKey{Provider: "p", Model: "m", TaskType: "code"}
	for i := 0; i < 5; i++ {
		if err := store.Record(RunSample{Key: key, InputTokens: 700, OutputTokens: 400, Success: true, Continued: i < 2, PreflightOK: true}); err != nil {
			t.Fatal(err)
		}
	}
	planner := DefaultAdaptivePlanner{}
	decision := planner.Recommend(AdaptiveInput{Key: key, EstimatedInputTokens: 800, EffectiveContext: 1200, Base: LoopOptions{MaxIterations: 4}}, store)
	if !decision.EnableChunking || !decision.EscalateTier || decision.ResponseTokenBudget > 280 {
		t.Fatalf("decision=%+v", decision)
	}
	toolDecision := planner.Recommend(AdaptiveInput{Key: key, EstimatedInputTokens: 800, EffectiveContext: 1200, HasTools: true}, store)
	if toolDecision.EnableChunking {
		t.Fatalf("tool turn was chunked: %+v", toolDecision)
	}
}

type sampleRecorder struct{ samples []RunSample }

func (r *sampleRecorder) Record(sample RunSample) error {
	r.samples = append(r.samples, sample)
	return nil
}

func TestRecordOutcomeWritesExpertAndStrategySamples(t *testing.T) {
	recorder := &sampleRecorder{}
	orchestrator := New(Config{Learning: LearningConfig{Recorder: recorder}})
	plan := &Plan{Expert: &ExpertDefinition{ID: "code"}, Tier: TierDecision{ToolCall: ModelTierFast, Synthesis: ModelTierStrong}}
	if err := orchestrator.RecordOutcome(plan, RunSample{Key: StatsKey{Provider: "p", Model: "m"}, Success: true, Latency: time.Second}); err != nil {
		t.Fatal(err)
	}
	if len(recorder.samples) != 2 || recorder.samples[0].Key.TaskType != "code" || recorder.samples[1].Key.Provider != "moe-engine" {
		t.Fatalf("samples=%+v", recorder.samples)
	}
}
