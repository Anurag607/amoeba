package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunListsCasesWithoutModelAccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--list", "--domains", "coding"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "coding-lock-release") || strings.Contains(stdout.String(), "safety-prompt-injection") {
		t.Fatalf("case list = %q", stdout.String())
	}
}

func TestRunRequiresModelAndRejectsBadMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), nil, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "--model") {
		t.Fatalf("missing model error = %v", err)
	}
	if err := run(context.Background(), []string{"--model", "m", "--mode", "invalid"}, &stdout, &stderr); err == nil {
		t.Fatal("invalid mode error = nil")
	}
}

func TestReportPathAndSafeName(t *testing.T) {
	if got := safeName("Llama 3.1:8B"); got != "llama-3-1-8b" {
		t.Fatalf("safeName() = %q", got)
	}
}
