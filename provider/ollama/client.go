// Package ollama provides bounded, read-only Ollama model discovery.
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anurgosw/agentic-moe/moe"
)

// Config controls the Ollama discovery client.
type Config struct {
	BaseURL       string
	AllowRemote   bool
	Timeout       time.Duration
	MaxBodyBytes  int64
	ContextWindow int
	MaxParallel   int
	HTTPClient    *http.Client
}

// Client discovers Ollama metadata without pulling, switching, or invoking models.
type Client struct {
	baseURL       *url.URL
	http          *http.Client
	timeout       time.Duration
	maxBodyBytes  int64
	contextWindow int
	maxParallel   int
}

// New validates the endpoint and creates a bounded client.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:11434"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 4 << 20
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = 8192
	}
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 1
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("ollama: parse base url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("ollama: base url scheme must be http or https")
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("ollama: base url must contain only scheme, host, port, and optional path")
	}
	if !cfg.AllowRemote && !isLoopback(u.Hostname()) {
		return nil, fmt.Errorf("ollama: remote endpoint requires allow_remote")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	client := &http.Client{}
	if cfg.HTTPClient != nil {
		*client = *cfg.HTTPClient
	}
	if client.Timeout <= 0 || client.Timeout > cfg.Timeout {
		client.Timeout = cfg.Timeout
	}
	// Discovery endpoints never need redirects. Rejecting them prevents a
	// loopback endpoint from becoming an SSRF trampoline to another host.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: u, http: client, timeout: cfg.Timeout, maxBodyBytes: cfg.MaxBodyBytes, contextWindow: cfg.ContextWindow, maxParallel: cfg.MaxParallel}, nil
}

// Name implements runtimekit.ProviderSource.
func (c *Client) Name() string { return "ollama" }

// Version returns the Ollama server version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var response struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/api/version", &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Version) == "" {
		return "", fmt.Errorf("ollama: version response omitted version")
	}
	return response.Version, nil
}

// Discover returns installed models as conservative routing candidates.
func (c *Client) Discover(ctx context.Context) ([]moe.ProviderCandidate, error) {
	if _, err := c.Version(ctx); err != nil {
		return nil, err
	}
	var response tagsResponse
	if err := c.get(ctx, "/api/tags", &response); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	candidates := make([]moe.ProviderCandidate, 0, len(response.Models))
	for _, model := range response.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		candidates = append(candidates, moe.ProviderCandidate{
			Provider: "ollama", Model: name, Tier: tierFor(model), ContextWindow: c.contextWindow,
			MaxParallel: c.maxParallel, Placement: "local", Ready: true,
			Resources: moe.ProviderResourceProfile{ResidentMemoryBytes: max64(model.Size, 0)},
			Health:    moe.ProviderHealth{ObservedAt: now, ExpiresAt: now.Add(30 * time.Second)},
		})
	}
	return candidates, nil
}

func (c *Client) get(ctx context.Context, endpoint string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("ollama: create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ollama: request %s returned %s", endpoint, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("ollama: read %s: %w", endpoint, err)
	}
	if int64(len(body)) > c.maxBodyBytes {
		return fmt.Errorf("ollama: response %s exceeds %d bytes", endpoint, c.maxBodyBytes)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("ollama: decode %s: %w", endpoint, err)
	}
	return nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func max64(value, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}
