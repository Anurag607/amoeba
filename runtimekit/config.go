// Package runtimekit composes agentic-moe into a batteries-included planning runtime.
package runtimekit

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Anurag607/amoeba/moe"
	"gopkg.in/yaml.v3"
)

// ConfigVersion is the latest supported on-disk configuration version.
const ConfigVersion = 1

// Config is the portable YAML or JSON configuration contract.
type Config struct {
	Version int            `json:"version" yaml:"version"`
	Runtime RuntimeConfig  `json:"runtime" yaml:"runtime"`
	Models  ModelConfig    `json:"models" yaml:"models"`
	Experts []ExpertConfig `json:"experts" yaml:"experts"`
	Ollama  OllamaConfig   `json:"ollama" yaml:"ollama"`
	HTTP    HTTPConfig     `json:"http" yaml:"http"`
}

// RuntimeConfig controls deterministic planner limits.
type RuntimeConfig struct {
	MaxIterations          int `json:"max_iterations" yaml:"max_iterations"`
	MaxToolCalls           int `json:"max_tool_calls" yaml:"max_tool_calls"`
	ResponseTokenBudget    int `json:"response_token_budget" yaml:"response_token_budget"`
	MaxInjectedSkillTokens int `json:"max_injected_skill_tokens" yaml:"max_injected_skill_tokens"`
}

// ModelConfig maps abstract planner tiers to host-owned model identifiers.
type ModelConfig struct {
	Fast     string `json:"fast" yaml:"fast"`
	Balanced string `json:"balanced" yaml:"balanced"`
	Strong   string `json:"strong" yaml:"strong"`
}

// ExpertConfig describes a declarative expert available through every SDK.
type ExpertConfig struct {
	ID               string   `json:"id" yaml:"id"`
	Name             string   `json:"name" yaml:"name"`
	Description      string   `json:"description" yaml:"description"`
	Keywords         []string `json:"keywords" yaml:"keywords"`
	NegativeKeywords []string `json:"negative_keywords,omitempty" yaml:"negative_keywords,omitempty"`
	Prompt           string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	DefaultTier      string   `json:"default_tier" yaml:"default_tier"`
	Priority         int      `json:"priority" yaml:"priority"`
	CanSynthesize    bool     `json:"can_synthesize,omitempty" yaml:"can_synthesize,omitempty"`
	MaxIterations    int      `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
	MaxToolCalls     int      `json:"max_tool_calls,omitempty" yaml:"max_tool_calls,omitempty"`
}

// OllamaConfig enables read-only discovery against an Ollama API.
type OllamaConfig struct {
	Enabled       bool     `json:"enabled" yaml:"enabled"`
	BaseURL       string   `json:"base_url" yaml:"base_url"`
	AllowRemote   bool     `json:"allow_remote" yaml:"allow_remote"`
	Timeout       Duration `json:"timeout" yaml:"timeout"`
	MaxBodyBytes  int64    `json:"max_body_bytes" yaml:"max_body_bytes"`
	ContextWindow int      `json:"context_window" yaml:"context_window"`
	MaxParallel   int      `json:"max_parallel" yaml:"max_parallel"`
}

// HTTPConfig controls the optional stateless MCP HTTP endpoint.
type HTTPConfig struct {
	Address        string   `json:"address" yaml:"address"`
	BearerTokenEnv string   `json:"bearer_token_env,omitempty" yaml:"bearer_token_env,omitempty"`
	AllowedHosts   []string `json:"allowed_hosts,omitempty" yaml:"allowed_hosts,omitempty"`
	AllowedOrigins []string `json:"allowed_origins,omitempty" yaml:"allowed_origins,omitempty"`
	MaxBodyBytes   int64    `json:"max_body_bytes" yaml:"max_body_bytes"`
	MaxConcurrent  int      `json:"max_concurrent" yaml:"max_concurrent"`
	RequestTimeout Duration `json:"request_timeout" yaml:"request_timeout"`
	ShutdownGrace  Duration `json:"shutdown_grace" yaml:"shutdown_grace"`
}

// Overrides are explicit CLI values applied after environment variables.
type Overrides struct {
	HTTPAddress string
}

// DefaultConfig returns a safe local configuration with general-purpose experts.
func DefaultConfig() Config {
	return Config{
		Version: ConfigVersion,
		Runtime: RuntimeConfig{MaxIterations: 8, MaxToolCalls: 12, ResponseTokenBudget: 4096, MaxInjectedSkillTokens: 4096},
		Experts: defaultExperts(),
		Ollama:  OllamaConfig{BaseURL: "http://127.0.0.1:11434", Timeout: Duration(5 * time.Second), MaxBodyBytes: 4 << 20, ContextWindow: 8192, MaxParallel: 1},
		HTTP:    HTTPConfig{Address: "127.0.0.1:8080", MaxBodyBytes: 1 << 20, MaxConcurrent: 32, RequestTimeout: Duration(30 * time.Second), ShutdownGrace: Duration(10 * time.Second)},
	}
}

// Load reads a strict YAML/JSON config and applies env then CLI overrides.
func Load(path string, getenv func(string) string, overrides Overrides) (Config, error) {
	cfg := DefaultConfig()
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := decode(body, &cfg); err != nil {
			return Config{}, err
		}
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	if overrides.HTTPAddress != "" {
		cfg.HTTP.Address = overrides.HTTPAddress
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Parse decodes strict YAML or JSON content, applies environment overrides, and validates it.
func Parse(body []byte, getenv func(string) string) (Config, error) {
	cfg := DefaultConfig()
	if err := decode(body, &cfg); err != nil {
		return Config{}, err
	}
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func decode(body []byte, cfg *Config) error {
	migrated, err := Migrate(body)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(migrated))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode config: multiple documents are not allowed")
		}
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

// Validate checks all configuration invariants without contacting providers.
func (c Config) Validate() error {
	if c.Version != ConfigVersion {
		return fmt.Errorf("config: unsupported version %d", c.Version)
	}
	if c.Runtime.MaxIterations <= 0 || c.Runtime.MaxToolCalls <= 0 || c.Runtime.ResponseTokenBudget <= 0 || c.Runtime.MaxInjectedSkillTokens <= 0 {
		return fmt.Errorf("config: runtime limits must be positive")
	}
	if len(c.Experts) == 0 {
		return fmt.Errorf("config: at least one expert is required")
	}
	seen := make(map[string]struct{}, len(c.Experts))
	var synthesizers int
	for i, e := range c.Experts {
		if !expertIDPattern.MatchString(e.ID) {
			return fmt.Errorf("config: expert %d has invalid id %q", i, e.ID)
		}
		if strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("config: expert %q requires a name", e.ID)
		}
		if e.MaxIterations < 0 || e.MaxToolCalls < 0 {
			return fmt.Errorf("config: expert %q limits cannot be negative", e.ID)
		}
		if _, ok := seen[e.ID]; ok {
			return fmt.Errorf("config: duplicate expert id %q", e.ID)
		}
		seen[e.ID] = struct{}{}
		if _, err := parseTier(e.DefaultTier); err != nil {
			return fmt.Errorf("config: expert %q: %w", e.ID, err)
		}
		if e.CanSynthesize {
			synthesizers++
		}
	}
	if synthesizers > 1 {
		return fmt.Errorf("config: at most one synthesis expert is allowed")
	}
	if c.Ollama.Timeout <= 0 || c.Ollama.MaxBodyBytes <= 0 || c.Ollama.ContextWindow <= 0 || c.Ollama.MaxParallel <= 0 {
		return fmt.Errorf("config: ollama limits must be positive")
	}
	ollamaURL, err := url.Parse(c.Ollama.BaseURL)
	if err != nil || (ollamaURL.Scheme != "http" && ollamaURL.Scheme != "https") || ollamaURL.Hostname() == "" || ollamaURL.User != nil || ollamaURL.RawQuery != "" || ollamaURL.Fragment != "" {
		return fmt.Errorf("config: ollama base_url must be an http(s) URL without credentials, query, or fragment")
	}
	if !c.Ollama.AllowRemote && !loopbackHost(ollamaURL.Hostname()) {
		return fmt.Errorf("config: remote ollama base_url requires allow_remote")
	}
	if strings.TrimSpace(c.HTTP.Address) == "" || c.HTTP.MaxBodyBytes <= 0 || c.HTTP.MaxConcurrent <= 0 || c.HTTP.RequestTimeout <= 0 || c.HTTP.ShutdownGrace <= 0 {
		return fmt.Errorf("config: http address and limits must be positive")
	}
	host, _, err := net.SplitHostPort(c.HTTP.Address)
	if err != nil {
		return fmt.Errorf("config: http address must be host:port: %w", err)
	}
	if !loopbackHost(host) && c.HTTP.BearerTokenEnv == "" {
		return fmt.Errorf("config: non-loopback http address requires bearer_token_env")
	}
	if (host == "" || net.ParseIP(host) != nil && net.ParseIP(host).IsUnspecified()) && len(c.HTTP.AllowedHosts) == 0 {
		return fmt.Errorf("config: wildcard http address requires allowed_hosts")
	}
	if c.HTTP.BearerTokenEnv != "" && !envNamePattern.MatchString(c.HTTP.BearerTokenEnv) {
		return fmt.Errorf("config: bearer_token_env must name an environment variable, not contain a secret")
	}
	for _, origin := range c.HTTP.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("config: invalid allowed origin %q", origin)
		}
	}
	for _, allowedHost := range c.HTTP.AllowedHosts {
		normalized := strings.Trim(strings.TrimSpace(allowedHost), "[]")
		if normalized == "" || strings.ContainsAny(normalized, "/@") || strings.Contains(normalized, ":") && net.ParseIP(normalized) == nil {
			return fmt.Errorf("config: invalid allowed host %q", allowedHost)
		}
	}
	return nil
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var expertIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func applyEnv(c *Config, getenv func(string) string) error {
	if v := getenv("AGENTIC_MOE_HTTP_ADDRESS"); v != "" {
		c.HTTP.Address = v
	}
	if v := getenv("AGENTIC_MOE_OLLAMA_URL"); v != "" {
		c.Ollama.BaseURL = v
	}
	if v := getenv("AGENTIC_MOE_OLLAMA_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("env AGENTIC_MOE_OLLAMA_ENABLED: %w", err)
		}
		c.Ollama.Enabled = enabled
	}
	return nil
}

func parseTier(value string) (moe.ModelTier, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "balanced":
		return moe.ModelTierBalanced, nil
	case "fast":
		return moe.ModelTierFast, nil
	case "strong":
		return moe.ModelTierStrong, nil
	default:
		return moe.ModelTierUnspecified, fmt.Errorf("unknown model tier %q", value)
	}
}

func defaultExperts() []ExpertConfig {
	return []ExpertConfig{
		{
			ID: "general", Name: "General Expert", Description: "General reasoning, writing, and clarification",
			Keywords:    []string{"explain", "help", "calculate", "arithmetic", "math", "logic", "reason", "reasoning", "write", "rewrite", "release note", "summarize", "summary", "clarify", "clarification", "diagnostic", "credential", "redact", "redacted", "secret", "safety"},
			Prompt:      "Reason carefully. Follow the requested output format exactly. Ask one concise question when essential information is missing.",
			DefaultTier: "balanced", Priority: 20,
		},
		{
			ID: "coding", Name: "Coding Expert", Description: "Software design, implementation, testing, and debugging",
			Keywords:    []string{"code", "coding", "bug", "debug", "test", "tests", "testing", "build", "api", "refactor", "refactoring", "function", "compile", "diff", "commit", "review", "pipeline", "ci", "database", "migration", "database migration"},
			Prompt:      "Identify the root cause, prefer the smallest safe change, and preserve exact output or API contracts.",
			DefaultTier: "balanced", Priority: 10,
		},
		{
			ID: "research", Name: "Research Expert", Description: "Evidence gathering and comparative analysis",
			Keywords:    []string{"research", "compare", "compares", "comparison", "source", "sources", "evidence", "analyze", "analysis", "study", "citation", "citations"},
			DefaultTier: "strong", Priority: 20,
		},
		{
			ID: "operations", Name: "Operations Expert", Description: "Deployment, reliability, incident, and rollback work",
			Keywords:    []string{"deploy", "deployment", "incident", "log", "logs", "production", "monitor", "monitored", "monitoring", "rollback", "rollout", "release", "reliability", "telemetry", "trace", "latency", "span"},
			Prompt:      "Fail closed on side effects. Require authority, preflight checks, rollback, and verification before claiming completion.",
			DefaultTier: "balanced", Priority: 20,
		},
		{
			ID: "synthesis", Name: "Synthesis Expert", Description: "Cross-domain planning and synthesis",
			Keywords:    []string{"plan", "planning", "architecture", "strategy", "cross domain", "tradeoff", "tradeoffs", "coordinate", "coordination", "migration"},
			Prompt:      "Integrate the relevant domains into one concise plan with dependencies, gates, rollback, and verification.",
			DefaultTier: "strong", Priority: 100, CanSynthesize: true,
		},
	}
}
