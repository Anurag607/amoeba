package runtimekit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anurgosw/agentic-moe/moe"
)

func TestLoadStrictPrecedenceAndMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("runtime:\n  max_iterations: 3\nhttp:\n  address: 127.0.0.1:9000\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(name string) string {
		if name == "AGENTIC_MOE_HTTP_ADDRESS" {
			return "127.0.0.1:9001"
		}
		return ""
	}
	cfg, err := Load(path, getenv, Overrides{HTTPAddress: "127.0.0.1:9002"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Version != ConfigVersion || cfg.Runtime.MaxIterations != 3 {
		t.Fatalf("Load() = version %d, iterations %d", cfg.Version, cfg.Runtime.MaxIterations)
	}
	if cfg.HTTP.Address != "127.0.0.1:9002" {
		t.Fatalf("HTTP address = %q", cfg.HTTP.Address)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	_, err := Parse([]byte("version: 1\nunknown: true\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("Parse() error = %v, want unknown field", err)
	}
}

func TestParseStrictBoundaries(t *testing.T) {
	t.Run("json duration", func(t *testing.T) {
		cfg, err := Parse([]byte(`{"version":1,"ollama":{"timeout":"250ms"}}`), nil)
		if err != nil || cfg.Ollama.Timeout != Duration(250000000) {
			t.Fatalf("Parse() = timeout %v, %v", cfg.Ollama.Timeout, err)
		}
	})
	for name, body := range map[string]string{
		"multiple documents": "version: 1\n---\nversion: 1\n",
		"duplicate key":      "version: 1\nversion: 1\n",
		"invalid env bool":   "version: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			getenv := func(string) string { return "" }
			if name == "invalid env bool" {
				getenv = func(key string) string {
					if key == "AGENTIC_MOE_OLLAMA_ENABLED" {
						return "sometimes"
					}
					return ""
				}
			}
			if _, err := Parse([]byte(body), getenv); err == nil {
				t.Fatal("Parse() error = nil")
			}
		})
	}
}

func TestConfigValidationRejectsUnsafeValues(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"unsupported version":   func(c *Config) { c.Version++ },
		"duplicate expert":      func(c *Config) { c.Experts = append(c.Experts, c.Experts[0]) },
		"malformed expert id":   func(c *Config) { c.Experts[0].ID = "Bad ID" },
		"negative expert limit": func(c *Config) { c.Experts[0].MaxIterations = -1 },
		"two synthesizers": func(c *Config) {
			c.Experts[0].CanSynthesize = true
			c.Experts[1].CanSynthesize = true
		},
		"remote ollama":         func(c *Config) { c.Ollama.BaseURL = "https://example.com" },
		"ollama credentials":    func(c *Config) { c.Ollama.BaseURL = "http://user@localhost:11434" },
		"wildcard without host": func(c *Config) { c.HTTP.Address = ":8080"; c.HTTP.BearerTokenEnv = "TOKEN" },
		"remote without token":  func(c *Config) { c.HTTP.Address = "192.0.2.1:8080" },
		"secret as env name":    func(c *Config) { c.HTTP.BearerTokenEnv = "actual secret" },
		"origin path":           func(c *Config) { c.HTTP.AllowedOrigins = []string{"https://example.com/path"} },
		"host with port":        func(c *Config) { c.HTTP.AllowedHosts = []string{"example.com:443"} },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestDefaultConfigJSONRoundTrip(t *testing.T) {
	body, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(body, nil)
	if err != nil || cfg.Version != ConfigVersion || len(cfg.Experts) == 0 {
		t.Fatalf("Parse(round trip) = %+v, %v", cfg, err)
	}
}

func FuzzParseConfig(f *testing.F) {
	f.Add([]byte("version: 1\n"))
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte("version: 1\nexperts: []\n"))
	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = Parse(body, func(string) string { return "" })
	})
}

func TestRuntimePlanAndHealth(t *testing.T) {
	runtime, err := New(DefaultConfig(), Options{ProviderSources: []ProviderSource{fakeSource{}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := runtime.Plan(context.Background(), "debug this code test failure", RoutingInput{HasCodeContext: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Expert.ID != "coding" {
		t.Fatalf("expert = %q, want coding", plan.Expert.ID)
	}
	if plan.Options.MaxIterations != DefaultConfig().Runtime.MaxIterations {
		t.Fatalf("max iterations = %d", plan.Options.MaxIterations)
	}
	report := runtime.Health(context.Background())
	if report.Status != "ok" || len(report.Providers) != 1 || !report.Providers[0].Ready {
		t.Fatalf("health = %+v", report)
	}
}

func TestRuntimeRejectsEmptyInputAndNegativeHistory(t *testing.T) {
	runtime, err := New(DefaultConfig(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Plan(context.Background(), " \t", RoutingInput{}); err == nil {
		t.Fatal("Plan(blank) error = nil")
	}
	if _, err := runtime.Plan(context.Background(), "task", RoutingInput{HistoryDepth: -1}); err == nil {
		t.Fatal("Plan(negative history) error = nil")
	}
	if _, err := runtime.PlanForExpert(context.Background(), " \n"); err == nil {
		t.Fatal("PlanForExpert(blank) error = nil")
	}
}

func TestRuntimePlansConcurrently(t *testing.T) {
	runtime, err := New(DefaultConfig(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 32)
	for i := 0; i < cap(errors); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := runtime.Plan(context.Background(), "research and compare code", RoutingInput{HasCodeContext: true})
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent Plan() error = %v", err)
		}
	}
}

type fakeSource struct{}

func (fakeSource) Name() string { return "fake" }
func (fakeSource) Discover(context.Context) ([]moe.ProviderCandidate, error) {
	return []moe.ProviderCandidate{{Provider: "fake", Model: "model", Tier: moe.ModelTierFast, ContextWindow: 1024, MaxParallel: 1, Placement: "local", Ready: true}}, nil
}
