package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersionAndConfigValidation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"version", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"version"`) {
		t.Fatalf("version output = %q", stdout.String())
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := run(context.Background(), []string{"config", "validate", "--json", path}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"valid": true`) {
		t.Fatalf("config output = %q", stdout.String())
	}
}

func TestRunInitDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	var stdout, stderr bytes.Buffer
	args := []string{"init", "--target", "generic", "--output", path}
	if err := run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), args, &stdout, &stderr); err == nil {
		t.Fatal("second init error = nil, want existing-file rejection")
	}
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"version", "extra"},
		{"schema", "extra"},
		{"doctor", "extra"},
		{"init", "extra"},
		{"mcp", "stdio", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatalf("run(%v) error = nil", args)
		}
	}
}

func TestRunSchemaDoctorAndCompletions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"schema"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil || schema["$schema"] == nil {
		t.Fatalf("schema = %+v, %v", schema, err)
	}

	stdout.Reset()
	if err := run(context.Background(), []string{"doctor", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.Status != "ok" {
		t.Fatalf("doctor = %+v, %v", report, err)
	}

	for _, shell := range []string{"bash", "zsh", "fish"} {
		stdout.Reset()
		if err := run(context.Background(), []string{"completion", shell}, &stdout, &stderr); err != nil || !strings.Contains(stdout.String(), "agentic-moe") {
			t.Fatalf("completion %s = %q, %v", shell, stdout.String(), err)
		}
	}
}

func TestRunPropagatesOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	if err := run(context.Background(), []string{"version"}, failingWriter{}, &stderr); err == nil {
		t.Fatal("run() error = nil")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
