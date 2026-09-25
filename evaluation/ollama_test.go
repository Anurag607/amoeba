package evaluation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaClientGeneratesBoundedChat(t *testing.T) {
	contract := mustJSONSchemaContract(`{"type":"object","properties":{"answer":{"type":"integer"}},"required":["answer"],"additionalProperties":false}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/base/api/chat" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s, content-type %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"stream":true`) || !strings.Contains(string(body), `"seed":7`) || !strings.Contains(string(body), `"num_ctx":4096`) || !strings.Contains(string(body), `"format":{"type":"object"`) {
			t.Errorf("body = %s", body)
		}
		_, _ = io.WriteString(w, `{"message":{"role":"assistant","content":"389"},"prompt_eval_count":5,"eval_count":1,"total_duration":1000000}`)
	}))
	defer server.Close()
	client, err := NewOllamaClient(OllamaConfig{BaseURL: server.URL + "/base", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Generate(context.Background(), GenerateRequest{Model: "model", System: "system", Prompt: "prompt", Seed: 7, MaxTokens: 5, ContextWindow: 4096, OutputContract: contract})
	if err != nil || response.Text != "389" || response.PromptTokens != 5 || response.CompletionTokens != 1 {
		t.Fatalf("response = %+v, error = %v", response, err)
	}
}

func TestOllamaClientDoesNotRepairOrRetryContractViolations(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"message":{"role":"assistant","content":"not-json"}}`)
	}))
	defer server.Close()
	client, err := NewOllamaClient(OllamaConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	contract := mustJSONSchemaContract(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`)
	response, err := client.Generate(context.Background(), GenerateRequest{Model: "m", Prompt: "p", MaxTokens: 8, OutputContract: contract})
	if err != nil || response.Text != "not-json" || calls != 1 {
		t.Fatalf("response = %+v, calls = %d, error = %v", response, calls, err)
	}
	invalid := OutputContract{Kind: OutputJSONSchema, Schema: []byte(`{`)}
	if _, err := client.Generate(context.Background(), GenerateRequest{Model: "m", Prompt: "p", MaxTokens: 8, OutputContract: invalid}); err == nil || calls != 1 {
		t.Fatalf("invalid contract error = %v, calls = %d", err, calls)
	}
}

func TestOllamaClientRejectsUnsafeEndpointsAndResponses(t *testing.T) {
	if _, err := NewOllamaClient(OllamaConfig{BaseURL: "https://example.com"}); err == nil {
		t.Fatal("remote endpoint error = nil")
	}
	targetReached := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetReached = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	client, err := NewOllamaClient(OllamaConfig{BaseURL: redirect.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(context.Background(), GenerateRequest{Model: "m", Prompt: "p", MaxTokens: 1})
	if err == nil || targetReached {
		t.Fatalf("redirect error = %v, target reached = %t", err, targetReached)
	}

	oversized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 100))
	}))
	defer oversized.Close()
	client, err = NewOllamaClient(OllamaConfig{BaseURL: oversized.URL, MaxBodyBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(context.Background(), GenerateRequest{Model: "m", Prompt: "p", MaxTokens: 1})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestOllamaClientPreservesCancellation(t *testing.T) {
	client, err := NewOllamaClient(OllamaConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Generate(ctx, GenerateRequest{Model: "m", Prompt: "p", MaxTokens: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestOllamaClientPreflightsModelWithoutLeakingBody(t *testing.T) {
	secret := "provider-secret-body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "available:1b") {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		http.Error(w, secret, http.StatusNotFound)
	}))
	defer server.Close()
	client, err := NewOllamaClient(OllamaConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Preflight(context.Background(), "missing:1b")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("preflight error = %v", err)
	}
	if err := client.Preflight(context.Background(), "available:1b"); err != nil {
		t.Fatalf("available preflight: %v", err)
	}
	if err := client.Preflight(context.Background(), "bad\nname"); err == nil {
		t.Fatal("invalid model error = nil")
	}
}
