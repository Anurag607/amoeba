package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/anurgosw/agentic-moe/evaluation"
	"github.com/anurgosw/agentic-moe/internal/buildinfo"
	"github.com/anurgosw/agentic-moe/runtimekit"
)

var errThreshold = errors.New("evaluation thresholds were not met")

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("agentic-moe-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "framework YAML or JSON configuration path")
	model := fs.String("model", "", "installed Ollama model to evaluate (required)")
	baseURL := fs.String("ollama-url", "", "Ollama base URL (defaults to framework config)")
	allowRemote := fs.Bool("allow-remote", false, "allow a non-loopback Ollama endpoint")
	repeats := fs.Int("repeats", 2, "trials per case and mode (1-20)")
	concurrency := fs.Int("concurrency", 1, "maximum concurrent model calls (1-32)")
	timeout := fs.Duration("timeout", 90*time.Second, "deadline for each model call")
	seed := fs.Int("seed", 42, "base deterministic seed")
	maxTokens := fs.Int("max-tokens", 256, "maximum generated tokens per trial")
	contextWindow := fs.Int("context-window", 0, "Ollama context window (default: framework config)")
	modeValue := fs.String("mode", "both", "baseline, moe, or both")
	domainValue := fs.String("domains", "", "comma-separated domain filter")
	reportPath := fs.String("json-report", "", "JSON report path (default: .eval/<timestamp>-<model>.json)")
	minAnswer := fs.Float64("min-answer-score", 0.70, "minimum aggregate MoE answer score")
	minRouting := fs.Float64("min-routing-score", 0.90, "minimum aggregate MoE routing score")
	list := fs.Bool("list", false, "list built-in cases without invoking a model")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	cases, err := evaluation.FilterCases(evaluation.BuiltInCases(), splitDomains(*domainValue))
	if err != nil {
		return err
	}
	if *list {
		return listCases(stdout, cases)
	}
	if strings.TrimSpace(*model) == "" {
		return fmt.Errorf("--model is required; use `ollama list` to choose an installed model")
	}
	modes, err := parseModes(*modeValue)
	if err != nil {
		return err
	}
	cfg, err := runtimekit.Load(*configPath, os.Getenv, runtimekit.Overrides{})
	if err != nil {
		return fmt.Errorf("load framework config: %w", err)
	}
	// A fixed model across modes makes the comparison measure planning and
	// expert prompting rather than model-size differences.
	cfg.Models.Fast, cfg.Models.Balanced, cfg.Models.Strong = *model, *model, *model
	planner, err := runtimekit.New(cfg, runtimekit.Options{})
	if err != nil {
		return fmt.Errorf("construct planner: %w", err)
	}
	defer func() { _ = planner.Close() }()
	endpoint := *baseURL
	if endpoint == "" {
		endpoint = cfg.Ollama.BaseURL
	}
	if *contextWindow == 0 {
		*contextWindow = cfg.Ollama.ContextWindow
	}
	generator, err := evaluation.NewOllamaClient(evaluation.OllamaConfig{
		BaseURL: endpoint, AllowRemote: *allowRemote || cfg.Ollama.AllowRemote,
		Timeout: *timeout, MaxBodyBytes: cfg.Ollama.MaxBodyBytes,
	})
	if err != nil {
		return err
	}
	if err := generator.Preflight(ctx, *model); err != nil {
		return err
	}
	info := buildinfo.Current()
	runner, err := evaluation.NewRunner(planner, generator, evaluation.Options{
		Model: *model, Modes: modes, Repeats: *repeats, Concurrency: *concurrency,
		Timeout: *timeout, Seed: *seed, MaxTokens: *maxTokens,
		ContextWindow:  *contextWindow,
		MinAnswerScore: *minAnswer, MinRoutingScore: *minRouting,
		FrameworkRevision: info.Version + "+" + info.Commit,
	})
	if err != nil {
		return err
	}
	report, runErr := runner.Run(ctx, cases)
	path := *reportPath
	if path == "" {
		path = defaultReportPath(*model, report.StartedAt)
	}
	if err := evaluation.WriteJSONFile(path, report); err != nil {
		return err
	}
	if err := evaluation.WriteText(stdout, report); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\nJSON report: %s\n", path); err != nil {
		return fmt.Errorf("write report path: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	if !report.Passed {
		if _, err := fmt.Fprintln(stderr, "agentic-moe-eval: evaluation thresholds were not met; inspect the report before changing cases or prompts"); err != nil {
			return fmt.Errorf("write threshold status: %w", err)
		}
		return errThreshold
	}
	return nil
}

func parseModes(value string) ([]evaluation.Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "both":
		return []evaluation.Mode{evaluation.ModeBaseline, evaluation.ModeMOE}, nil
	case "baseline":
		return []evaluation.Mode{evaluation.ModeBaseline}, nil
	case "moe":
		return []evaluation.Mode{evaluation.ModeMOE}, nil
	default:
		return nil, fmt.Errorf("--mode must be baseline, moe, or both")
	}
}

func splitDomains(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func listCases(w io.Writer, cases []evaluation.Case) error {
	sort.Slice(cases, func(i, j int) bool {
		if cases[i].Domain != cases[j].Domain {
			return cases[i].Domain < cases[j].Domain
		}
		return cases[i].ID < cases[j].ID
	})
	for _, item := range cases {
		if _, err := fmt.Fprintf(w, "%s\t%s\texpert=%s\n", item.Domain, item.ID, item.ExpectedExpert); err != nil {
			return fmt.Errorf("write case list: %w", err)
		}
	}
	return nil
}

func defaultReportPath(model string, started time.Time) string {
	return filepath.Join(".eval", started.UTC().Format("20060102T150405Z")+"-"+safeName(model)+".json")
}

func safeName(value string) string {
	var output strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_' {
			output.WriteRune(char)
			lastDash = char == '-'
		} else if output.Len() > 0 && !lastDash {
			output.WriteByte('-')
			lastDash = true
		}
	}
	name := strings.Trim(output.String(), "-")
	if name == "" {
		return "model"
	}
	return name
}
