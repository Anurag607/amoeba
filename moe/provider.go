package moe

import (
	"fmt"
	"strings"
	"time"
)

type ProviderResourceProfile struct {
	ResidentMemoryBytes int64         `json:"resident_memory_bytes,omitempty"`
	CacheBytesPerToken  int64         `json:"cache_bytes_per_token,omitempty"`
	ColdStart           time.Duration `json:"cold_start,omitempty"`
	Warmup              time.Duration `json:"warmup,omitempty"`
	Platforms           []string      `json:"platforms,omitempty"`
	BundleDigest        string        `json:"bundle_digest,omitempty"`
}

type ProviderHealth struct {
	ObservedAt time.Time `json:"observed_at,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	ReasonCode string    `json:"reason_code,omitempty"`
}

// ProviderCandidate describes routing-relevant capacity without embedding a
// concrete provider SDK or credential source.
type ProviderCandidate struct {
	Provider         string                  `json:"provider"`
	Model            string                  `json:"model"`
	Tier             ModelTier               `json:"tier"`
	ContextWindow    int                     `json:"context_window"`
	MaxParallel      int                     `json:"max_parallel"`
	ToolCalling      bool                    `json:"tool_calling"`
	StructuredOutput bool                    `json:"structured_output"`
	Placement        string                  `json:"placement"`
	Ready            bool                    `json:"ready"`
	AccountBinding   string                  `json:"account_binding,omitempty"`
	Resources        ProviderResourceProfile `json:"resources,omitempty"`
	Health           ProviderHealth          `json:"health,omitempty"`
}

type CandidateSource interface{ Candidates() []ProviderCandidate }

func (c ProviderCandidate) Validate() error {
	if strings.TrimSpace(c.Provider) == "" || strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("provider candidate requires provider and model")
	}
	if c.Tier < ModelTierFast || c.Tier > ModelTierStrong {
		return fmt.Errorf("provider candidate has invalid tier %d", c.Tier)
	}
	if c.ContextWindow <= 0 || c.MaxParallel <= 0 {
		return fmt.Errorf("provider candidate requires positive context window and parallelism")
	}
	if strings.TrimSpace(c.Placement) == "" {
		return fmt.Errorf("provider candidate requires placement")
	}
	if c.Resources.ResidentMemoryBytes < 0 || c.Resources.CacheBytesPerToken < 0 || c.Resources.ColdStart < 0 || c.Resources.Warmup < 0 {
		return fmt.Errorf("provider candidate resource values cannot be negative")
	}
	if !c.Health.ExpiresAt.IsZero() && !c.Health.ObservedAt.IsZero() && c.Health.ExpiresAt.Before(c.Health.ObservedAt) {
		return fmt.Errorf("provider health expiry precedes observation")
	}
	return nil
}
