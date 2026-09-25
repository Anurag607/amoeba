package evaluation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WriteJSON emits one indented report without additional prose.
func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("evaluation: encode JSON report: %w", err)
	}
	return nil
}

// WriteJSONFile writes a private report atomically and refuses to replace an
// existing path. This prevents report paths from being used as symlink targets.
func WriteJSONFile(path string, report Report) error {
	path = filepath.Clean(path)
	if path == "." || strings.TrimSpace(path) == "" {
		return fmt.Errorf("evaluation: report path is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("evaluation: report path already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("evaluation: inspect report path: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("evaluation: create report directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".agentic-moe-eval-*.tmp")
	if err != nil {
		return fmt.Errorf("evaluation: create report: %w", err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("evaluation: secure report: %w", err)
	}
	if err := WriteJSON(temp, report); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("evaluation: sync report: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("evaluation: close report: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		return fmt.Errorf("evaluation: commit report: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("evaluation: remove report staging file: %w", err)
	}
	committed = true
	return nil
}

// WriteText renders a concise scorecard suitable for a terminal or CI log.
func WriteText(w io.Writer, report Report) error {
	status := "FAIL"
	if report.Passed {
		status = "PASS"
	}
	if _, err := fmt.Fprintf(w, "agentic-moe capability evaluation: %s\nmodel: %s  cases: %d  trials: %d  repeats: %d  concurrency: %d (peak %d)\n\n", status, report.Model, report.CaseCount, report.TrialCount, report.Repeats, report.Concurrency, report.PeakConcurrency); err != nil {
		return fmt.Errorf("evaluation: write text report: %w", err)
	}
	if _, err := fmt.Fprintln(w, "Mode       Answer   Routing  Pass rate  Mean latency"); err != nil {
		return fmt.Errorf("evaluation: write mode heading: %w", err)
	}
	for _, summary := range report.Modes {
		routing := "n/a"
		if summary.RoutingTrials > 0 {
			routing = percent(summary.RoutingScore)
		}
		if _, err := fmt.Fprintf(w, "%-10s %-8s %-8s %-10s %dms\n", summary.Name, percent(summary.AnswerScore), routing, percent(summary.PassRate), summary.MeanLatencyMS); err != nil {
			return fmt.Errorf("evaluation: write mode summary: %w", err)
		}
	}
	if _, err := fmt.Fprintln(w, "\nDomain scorecard"); err != nil {
		return fmt.Errorf("evaluation: write domain heading: %w", err)
	}
	if _, err := fmt.Fprintln(w, "Mode       Domain             Answer   Routing  Pass rate"); err != nil {
		return fmt.Errorf("evaluation: write domain columns: %w", err)
	}
	for _, summary := range report.Domains {
		routing := "n/a"
		if summary.RoutingTrials > 0 {
			routing = percent(summary.RoutingScore)
		}
		if _, err := fmt.Fprintf(w, "%-10s %-18s %-8s %-8s %s\n", summary.Mode, summary.Domain, percent(summary.AnswerScore), routing, percent(summary.PassRate)); err != nil {
			return fmt.Errorf("evaluation: write domain summary: %w", err)
		}
	}
	if len(report.Failures) > 0 {
		if _, err := fmt.Fprintln(w, "\nFailure classes"); err != nil {
			return fmt.Errorf("evaluation: write failure classes heading: %w", err)
		}
		for _, category := range []FailureClass{FailureSafety, FailureFactual, FailureFormat, FailureInstruction} {
			if count := report.Failures[category]; count > 0 {
				if _, err := fmt.Fprintf(w, "- %s: %d trials\n", category, count); err != nil {
					return fmt.Errorf("evaluation: write failure class: %w", err)
				}
			}
		}
	}
	if _, err := fmt.Fprintln(w, "\nFailed trials"); err != nil {
		return fmt.Errorf("evaluation: write failure heading: %w", err)
	}
	failures := 0
	for _, result := range report.Results {
		if result.Passed {
			continue
		}
		failures++
		reason := result.Error
		if reason == "" {
			var failed []string
			for _, assertion := range result.Assertions {
				if !assertion.Passed {
					failed = append(failed, assertion.Name)
				}
			}
			if result.RoutingApplicable && result.RoutingScore < 1 {
				failed = append(failed, "routing")
			}
			reason = strings.Join(failed, ", ")
		}
		classes := ""
		if len(result.FailureClasses) > 0 {
			values := make([]string, len(result.FailureClasses))
			for index, category := range result.FailureClasses {
				values[index] = string(category)
			}
			classes = " [" + strings.Join(values, ", ") + "]"
		}
		if _, err := fmt.Fprintf(w, "- %s/%s #%d%s: %s\n", result.Mode, result.CaseID, result.Repeat, classes, reason); err != nil {
			return fmt.Errorf("evaluation: write trial failure: %w", err)
		}
	}
	if failures == 0 {
		if _, err := fmt.Fprintln(w, "- none"); err != nil {
			return fmt.Errorf("evaluation: write empty failures: %w", err)
		}
	}
	if _, err := fmt.Fprintln(w, "\nLifecycle probes"); err != nil {
		return fmt.Errorf("evaluation: write probe heading: %w", err)
	}
	for _, probe := range report.Probes {
		status := "PASS"
		if !probe.Passed {
			status = "FAIL"
		}
		if _, err := fmt.Fprintf(w, "- %s: %s (%s)\n", probe.Name, status, probe.Detail); err != nil {
			return fmt.Errorf("evaluation: write probe: %w", err)
		}
	}
	return nil
}

func percent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}
