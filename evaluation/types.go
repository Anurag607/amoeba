// Package evaluation provides provider-free routing regressions and an opt-in,
// provider-backed capability evaluator for agentic-moe plans. Live model calls
// never run as part of the offline test suite.
package evaluation

import (
	"context"
	"time"

	"github.com/anurgosw/agentic-moe/runtimekit"
)

// ReportVersion is the machine-readable evaluation report schema version.
const ReportVersion = 2

// Mode identifies the prompt strategy used for one trial.
type Mode string

const (
	ModeBaseline Mode = "baseline"
	ModeMOE      Mode = "moe"
)

// CheckKind identifies one deterministic answer assertion.
type CheckKind string

// FailureClass separates response-shape failures from content and safety
// failures without changing the strict assertion result.
type FailureClass string

const (
	CheckContainsAll CheckKind = "contains_all"
	CheckContainsAny CheckKind = "contains_any"
	CheckExcludesAll CheckKind = "excludes_all"
	CheckExact       CheckKind = "exact"
	CheckRegexp      CheckKind = "regexp"
	CheckJSONKeys    CheckKind = "json_keys"
	CheckMaxWords    CheckKind = "max_words"
	CheckQuestion    CheckKind = "question"

	FailureFactual     FailureClass = "factual"
	FailureFormat      FailureClass = "format"
	FailureInstruction FailureClass = "instruction"
	FailureSafety      FailureClass = "safety"
)

// Check is a deterministic rule applied to a model response.
type Check struct {
	Name     string       `json:"name"`
	Kind     CheckKind    `json:"kind"`
	Category FailureClass `json:"category"`
	Values   []string     `json:"values,omitempty"`
	Pattern  string       `json:"pattern,omitempty"`
}

// Case is one immutable capability probe. Prompts are intentionally omitted
// from reports so evaluation artifacts cannot become a prompt-data channel.
type Case struct {
	ID             string                  `json:"id"`
	Domain         string                  `json:"domain"`
	Prompt         string                  `json:"-"`
	Routing        runtimekit.RoutingInput `json:"-"`
	ExpectedExpert string                  `json:"expected_expert,omitempty"`
	Checks         []Check                 `json:"-"`
	MaxTokens      int                     `json:"-"`
}

// Planner is the narrow runtimekit surface used by the evaluator.
type Planner interface {
	Plan(context.Context, string, runtimekit.RoutingInput) (runtimekit.PlanView, error)
}

// GenerateRequest is a bounded, host-owned model call.
type GenerateRequest struct {
	Model         string
	System        string
	Prompt        string
	Seed          int
	MaxTokens     int
	ContextWindow int
}

// GenerateResponse contains only fields needed for scoring and telemetry.
type GenerateResponse struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalDuration    time.Duration
}

// Generator invokes a model provider. Implementations must honor context
// cancellation and must not log request or response bodies.
type Generator interface {
	Generate(context.Context, GenerateRequest) (GenerateResponse, error)
}

// Options controls one evaluation run.
type Options struct {
	Model             string
	Modes             []Mode
	Repeats           int
	Concurrency       int
	Timeout           time.Duration
	Seed              int
	MaxTokens         int
	ContextWindow     int
	MinAnswerScore    float64
	MinRoutingScore   float64
	PreviewRunes      int
	FrameworkRevision string
	Now               func() time.Time
}

// AssertionResult records one deterministic answer check.
type AssertionResult struct {
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	Category FailureClass `json:"category"`
	Passed   bool         `json:"passed"`
	Detail   string       `json:"detail,omitempty"`
}

// TrialResult is one case/mode/repeat outcome. It contains a bounded redacted
// preview and digest, never the prompt or an unbounded raw response.
type TrialResult struct {
	CaseID            string            `json:"case_id"`
	Domain            string            `json:"domain"`
	Mode              Mode              `json:"mode"`
	Repeat            int               `json:"repeat"`
	Model             string            `json:"model"`
	SelectedExpert    string            `json:"selected_expert,omitempty"`
	ExpectedExpert    string            `json:"expected_expert,omitempty"`
	ToolNames         []string          `json:"tool_names,omitempty"`
	Tier              string            `json:"tier,omitempty"`
	AnswerScore       float64           `json:"answer_score"`
	RoutingScore      float64           `json:"routing_score,omitempty"`
	RoutingApplicable bool              `json:"routing_applicable"`
	Passed            bool              `json:"passed"`
	Assertions        []AssertionResult `json:"assertions"`
	LatencyMS         int64             `json:"latency_ms"`
	PromptTokens      int               `json:"prompt_tokens,omitempty"`
	CompletionTokens  int               `json:"completion_tokens,omitempty"`
	OutputSHA256      string            `json:"output_sha256,omitempty"`
	OutputPreview     string            `json:"output_preview,omitempty"`
	ErrorClass        string            `json:"error_class,omitempty"`
	Error             string            `json:"error,omitempty"`
	FailureClasses    []FailureClass    `json:"failure_classes,omitempty"`
}

// Summary aggregates a mode or domain without hiding sample count.
type Summary struct {
	Name             string  `json:"name"`
	Mode             Mode    `json:"mode,omitempty"`
	Domain           string  `json:"domain,omitempty"`
	Trials           int     `json:"trials"`
	Passed           int     `json:"passed"`
	PassRate         float64 `json:"pass_rate"`
	AnswerScore      float64 `json:"answer_score"`
	RoutingScore     float64 `json:"routing_score,omitempty"`
	RoutingTrials    int     `json:"routing_trials,omitempty"`
	MeanLatencyMS    int64   `json:"mean_latency_ms"`
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
}

// ProbeResult records evaluator-level safety and lifecycle checks.
type ProbeResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Report is the stable, machine-readable result of one run.
type Report struct {
	SchemaVersion     int                  `json:"schema_version"`
	StartedAt         time.Time            `json:"started_at"`
	FinishedAt        time.Time            `json:"finished_at"`
	FrameworkRevision string               `json:"framework_revision,omitempty"`
	Model             string               `json:"model"`
	Seed              int                  `json:"seed"`
	Repeats           int                  `json:"repeats"`
	Concurrency       int                  `json:"concurrency"`
	PeakConcurrency   int                  `json:"peak_concurrency"`
	TimeoutMS         int64                `json:"timeout_ms"`
	ContextWindow     int                  `json:"context_window"`
	CaseCount         int                  `json:"case_count"`
	TrialCount        int                  `json:"trial_count"`
	Passed            bool                 `json:"passed"`
	MinAnswerScore    float64              `json:"min_answer_score"`
	MinRoutingScore   float64              `json:"min_routing_score"`
	Modes             []Summary            `json:"modes"`
	Domains           []Summary            `json:"domains"`
	Probes            []ProbeResult        `json:"probes"`
	Results           []TrialResult        `json:"results"`
	Failures          map[FailureClass]int `json:"failures,omitempty"`
	Limitations       []string             `json:"limitations"`
}
