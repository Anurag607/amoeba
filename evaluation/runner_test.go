package evaluation

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anurgosw/agentic-moe/runtimekit"
)

type fakePlanner struct{ err error }

func (p fakePlanner) Plan(_ context.Context, prompt string, _ runtimekit.RoutingInput) (runtimekit.PlanView, error) {
	if p.err != nil {
		return runtimekit.PlanView{}, p.err
	}
	expert := "general"
	if strings.Contains(prompt, "code") {
		expert = "coding"
	}
	return runtimekit.PlanView{
		Expert:    runtimekit.ExpertView{ID: expert, Name: expert, Description: "test expert"},
		Selection: runtimekit.SelectionView{Primary: expert}, Tier: runtimekit.TierView{Synthesis: "balanced"},
	}, nil
}

type fakeGenerator struct {
	active        atomic.Int64
	peak          atomic.Int64
	contextWindow atomic.Int64
	structured    atomic.Int64
}

func (g *fakeGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	g.contextWindow.Store(int64(request.ContextWindow))
	output := "PASS"
	if request.OutputContract.Kind == OutputJSONSchema {
		g.structured.Add(1)
		output = `{"value":"PASS"}`
	}
	select {
	case <-ctx.Done():
		return GenerateResponse{}, ctx.Err()
	default:
	}
	current := g.active.Add(1)
	updatePeak(&g.peak, current)
	defer g.active.Add(-1)
	time.Sleep(10 * time.Millisecond)
	return GenerateResponse{Text: output, PromptTokens: 3, CompletionTokens: 1}, nil
}

func TestRunnerComparesModesWithBoundedConcurrency(t *testing.T) {
	generator := &fakeGenerator{}
	runner, err := NewRunner(fakePlanner{}, generator, Options{
		Model: "model", Modes: []Mode{ModeBaseline, ModeMOE}, Repeats: 2, Concurrency: 2,
		Timeout: time.Second, Seed: 7, MaxTokens: 32, ContextWindow: 4096, MinAnswerScore: 1, MinRoutingScore: 1,
		Now: func() time.Time { return time.Unix(1, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{{ID: "code", Domain: "coding", Prompt: "code task", ExpectedExpert: "coding", Checks: []Check{{Name: "pass", Kind: CheckJSONKeys, Values: []string{"value"}}}, OutputContract: mustJSONSchemaContract(`{"type":"object","properties":{"value":{"type":"string"}}}`)}}
	report, err := runner.Run(context.Background(), cases)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || len(report.Results) != 4 || report.PeakConcurrency != 2 || len(report.Modes) != 2 || !report.Probes[0].Passed {
		t.Fatalf("report = %+v", report)
	}
	if generator.peak.Load() > 2 || generator.contextWindow.Load() != 4096 || report.ContextWindow != 4096 || generator.structured.Load() != 4 {
		t.Fatalf("provider peak = %d, context = %d, report context = %d, structured = %d", generator.peak.Load(), generator.contextWindow.Load(), report.ContextWindow, generator.structured.Load())
	}
	for _, result := range report.Results {
		if result.OutputContract != OutputJSONSchema {
			t.Fatalf("result output contract = %q", result.OutputContract)
		}
	}
}

func TestRunnerClassifiesPlanningFailure(t *testing.T) {
	runner, err := NewRunner(fakePlanner{err: errors.New("plan failed")}, &fakeGenerator{}, Options{
		Model: "model", Modes: []Mode{ModeMOE}, Repeats: 1, Concurrency: 1,
		Timeout: time.Second, MaxTokens: 4, MinAnswerScore: 1, MinRoutingScore: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := Case{ID: "x", Domain: "d", Prompt: "p", Checks: []Check{{Name: "pass", Kind: CheckExact, Values: []string{"PASS"}}}}
	report, err := runner.Run(context.Background(), []Case{item})
	if err != nil || report.Results[0].ErrorClass != "planning" || report.Passed {
		t.Fatalf("report = %+v, error = %v", report, err)
	}
}

func TestNewRunnerRejectsInvalidOptions(t *testing.T) {
	_, err := NewRunner(fakePlanner{}, &fakeGenerator{}, Options{})
	if err == nil {
		t.Fatal("invalid options error = nil")
	}
}

func TestMOESystemPromptEndsWithFormatGuard(t *testing.T) {
	prompt := moeSystemPrompt(runtimekit.PlanView{
		Expert:                   runtimekit.ExpertView{ID: "research", Name: "Research", Description: "evidence"},
		SystemPromptAugmentation: "Do not invent sources.",
	})
	if !strings.HasSuffix(prompt, "do not add labels, framing, or explanation unless requested.") {
		t.Fatalf("system prompt does not end with format guard: %q", prompt)
	}
	if inherited := moeSystemPrompt(runtimekit.PlanView{}); inherited != baselineSystemPrompt {
		t.Fatalf("empty expert guidance changed baseline: %q", inherited)
	}
}
