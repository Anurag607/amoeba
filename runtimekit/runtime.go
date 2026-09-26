package runtimekit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Anurag607/amoeba/moe"
	"github.com/Anurag607/amoeba/skills"
)

// ProviderSource discovers model candidates without transferring credentials.
type ProviderSource interface {
	Name() string
	Discover(context.Context) ([]moe.ProviderCandidate, error)
}

// Options supplies optional host-owned integrations.
type Options struct {
	Skills          *skills.Registry
	ProviderSources []ProviderSource
}

// Runtime is the production composition facade shared by SDK and MCP surfaces.
type Runtime struct {
	cfg       Config
	orch      *moe.Orchestrator
	experts   *moe.Registry
	providers []ProviderSource
	closeOnce sync.Once
}

// New validates config and constructs the canonical planner.
func New(cfg Config, opts Options) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	experts := moe.NewRegistry()
	for _, ec := range cfg.Experts {
		tier, _ := parseTier(ec.DefaultTier)
		experts.Register(&moe.ExpertDefinition{
			ID: moe.ExpertID(ec.ID), Name: ec.Name, Description: ec.Description,
			Keywords: append([]string(nil), ec.Keywords...), NegativeKeywords: append([]string(nil), ec.NegativeKeywords...),
			PromptAugmentation: ec.Prompt, DefaultTier: tier, Priority: ec.Priority,
			CanSynthesize: ec.CanSynthesize, MaxIterations: ec.MaxIterations, MaxToolCalls: ec.MaxToolCalls,
		})
	}
	if opts.Skills == nil {
		opts.Skills = skills.NewRegistry()
	}
	tiers := moe.TierMapping{moe.ModelTierFast: cfg.Models.Fast, moe.ModelTierBalanced: cfg.Models.Balanced, moe.ModelTierStrong: cfg.Models.Strong}
	orch := moe.New(moe.Config{
		Experts: experts, Skills: opts.Skills, Tiers: tiers,
		DefaultOptions:         moe.LoopOptions{MaxIterations: cfg.Runtime.MaxIterations, MaxToolCalls: cfg.Runtime.MaxToolCalls, ResponseTokenBudget: cfg.Runtime.ResponseTokenBudget},
		MaxInjectedSkillTokens: cfg.Runtime.MaxInjectedSkillTokens,
	})
	return &Runtime{cfg: cfg, orch: orch, experts: experts, providers: append([]ProviderSource(nil), opts.ProviderSources...)}, nil
}

// Validate validates the runtime's immutable configuration.
func (r *Runtime) Validate() error { return r.cfg.Validate() }

// Plan routes a task and returns a transport-safe plan.
func (r *Runtime) Plan(_ context.Context, query string, routing RoutingInput) (PlanView, error) {
	if strings.TrimSpace(query) == "" {
		return PlanView{}, fmt.Errorf("plan: query is required")
	}
	if routing.HistoryDepth < 0 {
		return PlanView{}, fmt.Errorf("plan: history depth cannot be negative")
	}
	plan, err := r.orch.Plan(query, routing.toMOE())
	if err != nil {
		return PlanView{}, fmt.Errorf("plan: %w", err)
	}
	return makePlanView(plan), nil
}

// PlanForExpert selects an expert explicitly inside the default fail-closed tool ceiling.
func (r *Runtime) PlanForExpert(_ context.Context, id string) (PlanView, error) {
	if strings.TrimSpace(id) == "" {
		return PlanView{}, fmt.Errorf("plan for expert: expert id is required")
	}
	plan, err := r.orch.PlanForExpert(moe.ExpertID(id))
	if err != nil {
		return PlanView{}, fmt.Errorf("plan for expert: %w", err)
	}
	return makePlanView(plan), nil
}

// Manifest returns the stable capabilities offered by the composition layer.
func (r *Runtime) Manifest() Manifest {
	experts := make([]ExpertView, 0, len(r.experts.All()))
	for _, e := range r.experts.All() {
		experts = append(experts, ExpertView{ID: string(e.ID), Name: e.Name, Description: e.Description, DefaultTier: e.DefaultTier.String(), CanSynthesize: e.CanSynthesize})
	}
	providers := make([]string, 0, len(r.providers))
	for _, source := range r.providers {
		providers = append(providers, source.Name())
	}
	sort.Strings(providers)
	return Manifest{SchemaVersion: ConfigVersion, Experts: experts, Providers: providers, SideEffectsEnabled: false, Transports: []string{"stdio", "streamable-http"}}
}

// Health checks configured read-only providers and reports degraded state without hiding it.
func (r *Runtime) Health(ctx context.Context) HealthReport {
	report := HealthReport{Status: "ok", CheckedAt: time.Now().UTC(), Providers: make([]ProviderStatus, 0, len(r.providers))}
	for _, source := range r.providers {
		candidates, err := source.Discover(ctx)
		status := ProviderStatus{Name: source.Name(), CandidateCount: len(candidates), Ready: err == nil}
		if err != nil {
			status.Error = err.Error()
			report.Status = "degraded"
		}
		report.Providers = append(report.Providers, status)
	}
	return report
}

// Close releases composition resources. It is idempotent.
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {})
	return nil
}
