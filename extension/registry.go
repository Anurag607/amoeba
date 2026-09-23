// Package extension provides rollback-aware lifecycle management for narrow
// provider, tool, context, and skill registries. It is not a general plugin SDK.
package extension

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateRegistered  State = "registered"
	StateActive      State = "active"
	StateUnhealthy   State = "unhealthy"
	StateQuarantined State = "quarantined"
)

type Trust string

const (
	TrustBuiltin    Trust = "builtin"
	TrustHost       Trust = "host"
	TrustThirdParty Trust = "third_party"
)

type Contribution[T any] struct {
	ID           string
	Version      string
	Owner        string
	Trust        Trust
	Dependencies []string
	Value        T
	Activate     func(context.Context, T) error
	Deactivate   func(context.Context, T) error
	Health       func(context.Context, T) error
}

type Status struct {
	ID           string    `json:"id"`
	Version      string    `json:"version"`
	Owner        string    `json:"owner"`
	Trust        Trust     `json:"trust"`
	State        State     `json:"state"`
	LastError    string    `json:"last_error,omitempty"`
	LastHealthAt time.Time `json:"last_health_at,omitempty"`
}

type entry[T any] struct {
	contribution Contribution[T]
	status       Status
}
type Registry[T any] struct {
	mu      sync.Mutex
	entries map[string]*entry[T]
}

func NewRegistry[T any]() *Registry[T] { return &Registry[T]{entries: make(map[string]*entry[T])} }

func (r *Registry[T]) Register(contribution Contribution[T]) error {
	if strings.TrimSpace(contribution.ID) == "" || strings.TrimSpace(contribution.Version) == "" || strings.TrimSpace(contribution.Owner) == "" {
		return fmt.Errorf("extension requires id, version, and owner")
	}
	if contribution.Trust == "" {
		contribution.Trust = TrustThirdParty
	}
	if contribution.Trust != TrustBuiltin && contribution.Trust != TrustHost && contribution.Trust != TrustThirdParty {
		return fmt.Errorf("extension %q has invalid trust %q", contribution.ID, contribution.Trust)
	}
	seenDependencies := make(map[string]struct{}, len(contribution.Dependencies))
	for _, dependency := range contribution.Dependencies {
		if strings.TrimSpace(dependency) == "" || dependency == contribution.ID {
			return fmt.Errorf("extension %q has invalid dependency %q", contribution.ID, dependency)
		}
		if _, duplicate := seenDependencies[dependency]; duplicate {
			return fmt.Errorf("extension %q repeats dependency %q", contribution.ID, dependency)
		}
		seenDependencies[dependency] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[contribution.ID]; exists {
		return fmt.Errorf("extension %q already registered", contribution.ID)
	}
	contribution.Dependencies = append([]string(nil), contribution.Dependencies...)
	r.entries[contribution.ID] = &entry[T]{contribution: contribution, status: Status{ID: contribution.ID, Version: contribution.Version, Owner: contribution.Owner, Trust: contribution.Trust, State: StateRegistered}}
	return nil
}

// ActivateAll activates a dependency-ordered list atomically. Earlier
// activations are rolled back if a later contribution fails.
func (r *Registry[T]) ActivateAll(ctx context.Context, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ordered, err := r.activationOrder(ids)
	if err != nil {
		return err
	}
	activated := make([]*entry[T], 0, len(ordered))
	for _, id := range ordered {
		if err := ctx.Err(); err != nil {
			r.rollback(ctx, activated)
			return err
		}
		item, ok := r.entries[id]
		if !ok {
			r.rollback(ctx, activated)
			return fmt.Errorf("extension %q not registered", id)
		}
		if item.status.State == StateQuarantined {
			r.rollback(ctx, activated)
			return fmt.Errorf("extension %q is quarantined", id)
		}
		if item.status.State == StateActive {
			continue
		}
		for _, dependency := range item.contribution.Dependencies {
			dep, ok := r.entries[dependency]
			if !ok || dep.status.State != StateActive {
				r.rollback(ctx, activated)
				return fmt.Errorf("extension %q dependency %q is not active", id, dependency)
			}
		}
		if item.contribution.Activate != nil {
			if err := item.contribution.Activate(ctx, item.contribution.Value); err != nil {
				item.status.State, item.status.LastError = StateUnhealthy, err.Error()
				r.rollback(ctx, activated)
				return fmt.Errorf("activate extension %q: %w", id, err)
			}
		}
		item.status.State, item.status.LastError = StateActive, ""
		activated = append(activated, item)
	}
	return nil
}

func (r *Registry[T]) activationOrder(ids []string) ([]string, error) {
	state := make(map[string]uint8)
	ordered := make([]string, 0)
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("extension dependency cycle at %q", id)
		}
		if state[id] == 2 {
			return nil
		}
		item, ok := r.entries[id]
		if !ok {
			return fmt.Errorf("extension %q not registered", id)
		}
		state[id] = 1
		for _, dependency := range item.contribution.Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func (r *Registry[T]) rollback(ctx context.Context, activated []*entry[T]) {
	for i := len(activated) - 1; i >= 0; i-- {
		item := activated[i]
		if item.contribution.Deactivate != nil {
			_ = item.contribution.Deactivate(ctx, item.contribution.Value)
		}
		item.status.State = StateRegistered
	}
}

func (r *Registry[T]) Quarantine(id, reason string) error {
	return r.QuarantineContext(context.Background(), id, reason)
}

// QuarantineContext disables an active contribution before excluding it from
// future snapshots.
func (r *Registry[T]) QuarantineContext(ctx context.Context, id, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("extension %q not registered", id)
	}
	var deactivateErr error
	if item.status.State == StateActive && item.contribution.Deactivate != nil {
		deactivateErr = item.contribution.Deactivate(ctx, item.contribution.Value)
	}
	item.status.State, item.status.LastError = StateQuarantined, reason
	if deactivateErr != nil {
		item.status.LastError += "; deactivate: " + deactivateErr.Error()
		return fmt.Errorf("quarantine extension %q: %w", id, deactivateErr)
	}
	return nil
}

func (r *Registry[T]) Statuses() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Status, 0, len(r.entries))
	for _, item := range r.entries {
		out = append(out, item.status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CheckHealth probes every active extension. Failures are deactivated and
// quarantined so later snapshots cannot keep exposing a broken capability.
func (r *Registry[T]) CheckHealth(ctx context.Context) []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.entries {
		if item.status.State != StateActive || item.contribution.Health == nil {
			continue
		}
		item.status.LastHealthAt = time.Now()
		if err := item.contribution.Health(ctx, item.contribution.Value); err != nil {
			if item.contribution.Deactivate != nil {
				_ = item.contribution.Deactivate(ctx, item.contribution.Value)
			}
			item.status.State, item.status.LastError = StateQuarantined, err.Error()
		}
	}
	return r.statusesLocked()
}

// Update replaces one contribution transactionally. The previous active
// version is restored if the replacement cannot activate.
func (r *Registry[T]) Update(ctx context.Context, replacement Contribution[T]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.entries[replacement.ID]
	if !ok {
		return fmt.Errorf("extension %q not registered", replacement.ID)
	}
	if replacement.Version == "" || replacement.Owner != current.contribution.Owner {
		return fmt.Errorf("extension update requires version and the same owner")
	}
	if replacement.Trust == "" {
		replacement.Trust = current.contribution.Trust
	}
	seen := make(map[string]struct{}, len(replacement.Dependencies))
	for _, dependency := range replacement.Dependencies {
		if dependency == replacement.ID || dependency == "" {
			return fmt.Errorf("extension %q has invalid dependency %q", replacement.ID, dependency)
		}
		if _, duplicate := seen[dependency]; duplicate {
			return fmt.Errorf("extension %q repeats dependency %q", replacement.ID, dependency)
		}
		seen[dependency] = struct{}{}
	}
	probe := &entry[T]{contribution: replacement, status: current.status}
	r.entries[replacement.ID] = probe
	_, validationErr := r.activationOrder([]string{replacement.ID})
	r.entries[replacement.ID] = current
	if validationErr != nil {
		return validationErr
	}
	wasActive := current.status.State == StateActive
	if wasActive && current.contribution.Deactivate != nil {
		if err := current.contribution.Deactivate(ctx, current.contribution.Value); err != nil {
			return fmt.Errorf("deactivate extension %q for update: %w", replacement.ID, err)
		}
	}
	next := &entry[T]{contribution: replacement, status: Status{ID: replacement.ID, Version: replacement.Version, Owner: replacement.Owner, Trust: replacement.Trust, State: StateRegistered}}
	if wasActive && replacement.Activate != nil {
		if err := replacement.Activate(ctx, replacement.Value); err != nil {
			if current.contribution.Activate != nil {
				_ = current.contribution.Activate(ctx, current.contribution.Value)
			}
			return fmt.Errorf("activate extension %q update: %w", replacement.ID, err)
		}
		next.status.State = StateActive
	}
	r.entries[replacement.ID] = next
	return nil
}

func (r *Registry[T]) Unregister(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("extension %q not registered", id)
	}
	for otherID, other := range r.entries {
		for _, dependency := range other.contribution.Dependencies {
			if dependency == id {
				return fmt.Errorf("extension %q is required by %q", id, otherID)
			}
		}
	}
	if item.status.State == StateActive && item.contribution.Deactivate != nil {
		if err := item.contribution.Deactivate(ctx, item.contribution.Value); err != nil {
			return fmt.Errorf("deactivate extension %q: %w", id, err)
		}
	}
	delete(r.entries, id)
	return nil
}

func (r *Registry[T]) statusesLocked() []Status {
	out := make([]Status, 0, len(r.entries))
	for _, item := range r.entries {
		out = append(out, item.status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ActiveSnapshot returns stable active contributions for one admitted run.
func (r *Registry[T]) ActiveSnapshot() map[string]Contribution[T] {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]Contribution[T])
	for id, item := range r.entries {
		if item.status.State == StateActive {
			contribution := item.contribution
			contribution.Dependencies = append([]string(nil), contribution.Dependencies...)
			out[id] = contribution
		}
	}
	return out
}
