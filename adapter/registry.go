package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// HarnessRegistry activates only explicitly named and host-allowlisted CLI
// adapters. Discovery never turns an arbitrary executable into authority.
type HarnessRegistry struct {
	mu        sync.RWMutex
	allowlist map[string]struct{}
	harnesses map[string]AgentHarness
	configs   map[string]HarnessConfig
}

func NewHarnessRegistry(allowlist ...string) *HarnessRegistry {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		if name = strings.TrimSpace(name); name != "" {
			allowed[name] = struct{}{}
		}
	}
	return &HarnessRegistry{allowlist: allowed, harnesses: make(map[string]AgentHarness), configs: make(map[string]HarnessConfig)}
}

func (r *HarnessRegistry) Register(harness AgentHarness) error {
	if harness == nil || strings.TrimSpace(harness.Name()) == "" {
		return fmt.Errorf("harness name is required")
	}
	name := harness.Name()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, allowed := r.allowlist[name]; !allowed {
		return fmt.Errorf("harness %q is not allowlisted", name)
	}
	if _, exists := r.harnesses[name]; exists {
		return fmt.Errorf("harness %q already registered", name)
	}
	r.harnesses[name] = harness
	return nil
}

func (r *HarnessRegistry) Configure(ctx context.Context, name string, config HarnessConfig) error {
	harness, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("harness %q not registered", name)
	}
	if config.Name != name {
		return fmt.Errorf("harness config name mismatch")
	}
	if err := harness.Validate(config); err != nil {
		return fmt.Errorf("validate harness %q: %w", name, err)
	}
	if err := harness.Configure(ctx, config); err != nil {
		return err
	}
	r.mu.Lock()
	r.configs[name] = cloneHarnessConfig(config)
	r.mu.Unlock()
	return nil
}

func (r *HarnessRegistry) Manifest(ctx context.Context, name string) (HarnessManifest, error) {
	harness, ok := r.Get(name)
	if !ok {
		return HarnessManifest{}, fmt.Errorf("harness %q not registered", name)
	}
	r.mu.RLock()
	config, configured := r.configs[name]
	r.mu.RUnlock()
	if !configured {
		return HarnessManifest{}, fmt.Errorf("harness %q is not configured", name)
	}
	version, err := harness.Version(ctx)
	if err != nil {
		return HarnessManifest{}, fmt.Errorf("version harness %q: %w", name, err)
	}
	health, err := harness.Health(ctx)
	if err != nil {
		return HarnessManifest{}, fmt.Errorf("health harness %q: %w", name, err)
	}
	var capabilities HarnessCapabilities
	if reporter, ok := harness.(HarnessCapabilityReporter); ok {
		capabilities, err = reporter.Capabilities(ctx)
		if err != nil {
			return HarnessManifest{}, fmt.Errorf("capabilities harness %q: %w", name, err)
		}
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return HarnessManifest{}, err
	}
	sum := sha256.Sum256(payload)
	return HarnessManifest{
		Name: name, Version: version, ConfigDigest: "sha256:" + hex.EncodeToString(sum[:]), Health: health,
		Capabilities: capabilities,
	}, nil
}

func cloneHarnessConfig(in HarnessConfig) HarnessConfig {
	in.Arguments = append([]string(nil), in.Arguments...)
	in.AllowEnv = append([]string(nil), in.AllowEnv...)
	if in.Metadata != nil {
		metadata := make(map[string]string, len(in.Metadata))
		for key, value := range in.Metadata {
			metadata[key] = value
		}
		in.Metadata = metadata
	}
	return in
}

func (r *HarnessRegistry) Get(name string) (AgentHarness, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	harness, ok := r.harnesses[name]
	return harness, ok
}

func (r *HarnessRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.harnesses))
	for name := range r.harnesses {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *HarnessRegistry) Statuses(ctx context.Context) ([]HarnessStatus, error) {
	names := r.List()
	out := make([]HarnessStatus, 0, len(names))
	for _, name := range names {
		harness, _ := r.Get(name)
		status, err := harness.Status(ctx)
		if err != nil {
			return nil, fmt.Errorf("status harness %q: %w", name, err)
		}
		out = append(out, status)
	}
	return out, nil
}
