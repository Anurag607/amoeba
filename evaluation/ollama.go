package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ollamaModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,255}$`)

// OllamaConfig bounds the opt-in model calls made by the evaluator.
type OllamaConfig struct {
	BaseURL      string
	AllowRemote  bool
	Timeout      time.Duration
	MaxBodyBytes int64
	HTTPClient   *http.Client
}

// OllamaClient implements Generator through Ollama's non-streaming chat API.
type OllamaClient struct {
	baseURL     *url.URL
	http        *http.Client
	timeout     time.Duration
	maxBodySize int64
}

// NewOllamaClient constructs a bounded client. Remote endpoints and redirects
// are denied by default so an evaluation cannot silently cross trust domains.
func NewOllamaClient(cfg OllamaConfig) (*OllamaClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "http://127.0.0.1:11434"
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("evaluation: Ollama URL must be http(s) without credentials, query, or fragment")
	}
	if !cfg.AllowRemote && !isLoopback(u.Hostname()) {
		return nil, fmt.Errorf("evaluation: remote Ollama URL requires explicit opt-in")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 8 << 20
	}
	client := &http.Client{}
	if cfg.HTTPClient != nil {
		clone := *cfg.HTTPClient
		client = &clone
	}
	if client.Timeout <= 0 || client.Timeout > cfg.Timeout {
		client.Timeout = cfg.Timeout
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return fmt.Errorf("evaluation: Ollama redirects are disabled")
	}
	return &OllamaClient{baseURL: u, http: client, timeout: cfg.Timeout, maxBodySize: cfg.MaxBodyBytes}, nil
}

// Generate performs one non-streaming, deterministic-temperature chat call.
func (c *OllamaClient) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	if !ollamaModelPattern.MatchString(request.Model) {
		return GenerateResponse{}, fmt.Errorf("evaluation: model name is invalid")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return GenerateResponse{}, fmt.Errorf("evaluation: prompt is required")
	}
	if request.MaxTokens <= 0 {
		return GenerateResponse{}, fmt.Errorf("evaluation: max tokens must be positive")
	}
	if request.ContextWindow == 0 {
		request.ContextWindow = 8192
	}
	if request.ContextWindow < 512 || request.ContextWindow > 1<<20 {
		return GenerateResponse{}, fmt.Errorf("evaluation: context window must be between 512 and 1048576")
	}
	if err := request.OutputContract.Validate(); err != nil {
		return GenerateResponse{}, fmt.Errorf("evaluation: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var format json.RawMessage
	if request.OutputContract.Kind == OutputJSONSchema {
		format = append(json.RawMessage(nil), request.OutputContract.Schema...)
	}
	body, err := json.Marshal(chatRequest{
		Model: request.Model, Stream: false, Think: false,
		Messages: []chatMessage{{Role: "system", Content: request.System}, {Role: "user", Content: request.Prompt}},
		Format:   format, Options: chatOptions{Temperature: 0, Seed: request.Seed, NumPredict: request.MaxTokens, NumCtx: request.ContextWindow},
	})
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("evaluation: encode Ollama request: %w", err)
	}
	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/chat"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("evaluation: create Ollama request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(httpRequest)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("evaluation: Ollama chat request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return GenerateResponse{}, fmt.Errorf("evaluation: Ollama chat returned %s", response.Status)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, c.maxBodySize+1))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("evaluation: read Ollama response: %w", err)
	}
	if int64(len(responseBody)) > c.maxBodySize {
		return GenerateResponse{}, fmt.Errorf("evaluation: Ollama response exceeds %d bytes", c.maxBodySize)
	}
	var decoded chatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return GenerateResponse{}, fmt.Errorf("evaluation: decode Ollama response: %w", err)
	}
	if strings.TrimSpace(decoded.Message.Content) == "" {
		return GenerateResponse{}, fmt.Errorf("evaluation: Ollama returned an empty response")
	}
	return GenerateResponse{
		Text: decoded.Message.Content, PromptTokens: decoded.PromptEvalCount,
		CompletionTokens: decoded.EvalCount, TotalDuration: time.Duration(decoded.TotalDuration),
	}, nil
}

// Preflight verifies that Ollama can resolve the model manifest before a suite
// spends many trials rediscovering a stale or incomplete local installation.
func (c *OllamaClient) Preflight(ctx context.Context, model string) error {
	if !ollamaModelPattern.MatchString(model) {
		return fmt.Errorf("evaluation: model name is invalid")
	}
	body, err := json.Marshal(struct {
		Model string `json:"model"`
	}{Model: model})
	if err != nil {
		return fmt.Errorf("evaluation: encode Ollama preflight: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/show"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("evaluation: create Ollama preflight: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("evaluation: Ollama model preflight: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("evaluation: Ollama cannot load model %q (%s)", model, response.Status)
	}
	read, err := io.Copy(io.Discard, io.LimitReader(response.Body, c.maxBodySize+1))
	if err != nil {
		return fmt.Errorf("evaluation: read Ollama preflight: %w", err)
	}
	if read > c.maxBodySize {
		return fmt.Errorf("evaluation: Ollama preflight response exceeds %d bytes", c.maxBodySize)
	}
	return nil
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Stream   bool            `json:"stream"`
	Think    bool            `json:"think"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  chatOptions     `json:"options"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatOptions struct {
	Temperature float64 `json:"temperature"`
	Seed        int     `json:"seed"`
	NumPredict  int     `json:"num_predict"`
	NumCtx      int     `json:"num_ctx"`
}

type chatResponse struct {
	Message         chatMessage `json:"message"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
	TotalDuration   int64       `json:"total_duration"`
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
