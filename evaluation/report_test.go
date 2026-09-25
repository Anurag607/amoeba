package evaluation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportWritersAreSafeAndRefuseReplacement(t *testing.T) {
	report := Report{SchemaVersion: ReportVersion, Model: "model", Passed: true, Modes: []Summary{{Name: "moe", Mode: ModeMOE, Trials: 1, Passed: 1, PassRate: 1, AnswerScore: 1}}}
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	if err := WriteJSONFile(path, report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %v, error = %v", info.Mode().Perm(), err)
	}
	if err := WriteJSONFile(path, report); err == nil {
		t.Fatal("replacement error = nil")
	}
	var output bytes.Buffer
	if err := WriteText(&output, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "PASS") || !strings.Contains(output.String(), "Domain scorecard") {
		t.Fatalf("text report = %q", output.String())
	}
}
