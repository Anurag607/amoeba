// Package policy provides layered, auditable capability decisions. Discovery
// and execution use the same immutable snapshot, but execution must evaluate
// the policy again immediately before a side effect.
package policy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Effect is the policy outcome for one action/resource pair.
type Effect string

const (
	EffectDeny    Effect = "deny"
	EffectApprove Effect = "require_approval"
	EffectAllow   Effect = "allow"
)

// Layer identifies the policy source responsible for a rule.
type Layer string

const (
	LayerGlobal    Layer = "global"
	LayerProvider  Layer = "provider"
	LayerModel     Layer = "model"
	LayerExpert    Layer = "expert"
	LayerPrincipal Layer = "principal"
	LayerSession   Layer = "session"
	LayerWorkspace Layer = "workspace"
	LayerParent    Layer = "parent"
	LayerSandbox   Layer = "sandbox"
)

// Rule applies to one action and optional resource. A trailing * is a prefix
// match; an empty resource matches every resource.
type Rule struct {
	Action     string `json:"action"`
	Resource   string `json:"resource,omitempty"`
	Effect     Effect `json:"effect"`
	ReasonCode string `json:"reason_code"`
	Immutable  bool   `json:"immutable,omitempty"`
}

// RuleSet is one ordered policy layer.
type RuleSet struct {
	Layer Layer  `json:"layer"`
	Rules []Rule `json:"rules"`
}

// Decision is the auditable effective result.
type Decision struct {
	Effect     Effect `json:"effect"`
	Layer      Layer  `json:"layer,omitempty"`
	ReasonCode string `json:"reason_code"`
	Action     string `json:"action"`
	Resource   string `json:"resource,omitempty"`
	Immutable  bool   `json:"immutable,omitempty"`
}

// Snapshot is an immutable policy projection for one admitted run.
type Snapshot struct {
	version string
	layers  []RuleSet
}

// NewSnapshot validates and deep-copies policy layers.
func NewSnapshot(version string, layers ...RuleSet) (Snapshot, error) {
	if strings.TrimSpace(version) == "" {
		return Snapshot{}, fmt.Errorf("policy version is required")
	}
	copyLayers := make([]RuleSet, len(layers))
	for i, layer := range layers {
		if layer.Layer == "" {
			return Snapshot{}, fmt.Errorf("policy layer %d has no name", i)
		}
		copyLayers[i] = RuleSet{Layer: layer.Layer, Rules: append([]Rule(nil), layer.Rules...)}
		for j, rule := range copyLayers[i].Rules {
			if strings.TrimSpace(rule.Action) == "" {
				return Snapshot{}, fmt.Errorf("policy %s rule %d has no action", layer.Layer, j)
			}
			switch rule.Effect {
			case EffectAllow, EffectApprove, EffectDeny:
			default:
				return Snapshot{}, fmt.Errorf("policy %s rule %d has invalid effect %q", layer.Layer, j, rule.Effect)
			}
			if strings.TrimSpace(rule.ReasonCode) == "" {
				return Snapshot{}, fmt.Errorf("policy %s rule %d has no reason code", layer.Layer, j)
			}
		}
	}
	return Snapshot{version: version, layers: copyLayers}, nil
}

// Version returns the immutable admission version.
func (s Snapshot) Version() string { return s.version }

// RuleSets returns a deep copy for audit or persistence.
func (s Snapshot) RuleSets() []RuleSet {
	out := make([]RuleSet, len(s.layers))
	for i, layer := range s.layers {
		out[i] = RuleSet{Layer: layer.Layer, Rules: append([]Rule(nil), layer.Rules...)}
	}
	return out
}

func (s Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Version string    `json:"version"`
		Layers  []RuleSet `json:"layers"`
	}{Version: s.version, Layers: s.RuleSets()})
}

// Decide evaluates every matching rule. Deny always wins; otherwise approval
// wins over allow. No matching rule fails closed.
func (s Snapshot) Decide(action, resource string) Decision {
	decision := Decision{Effect: EffectDeny, ReasonCode: "policy.no_matching_allow", Action: action, Resource: resource}
	matched := false
	for _, layer := range s.layers {
		for _, rule := range layer.Rules {
			if !matches(rule.Action, action) || !matchesResource(rule.Resource, resource) {
				continue
			}
			matched = true
			candidate := Decision{Effect: rule.Effect, Layer: layer.Layer, ReasonCode: rule.ReasonCode,
				Action: action, Resource: resource, Immutable: rule.Immutable}
			if rule.Effect == EffectDeny {
				return candidate
			}
			if decision.Effect != EffectApprove || rule.Effect == EffectApprove {
				decision = candidate
			}
		}
	}
	if !matched {
		return decision
	}
	return decision
}

// AllowedActions filters candidate action names before model discovery.
func (s Snapshot) AllowedActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if decision := s.Decide(action, ""); decision.Effect != EffectDeny {
			out = append(out, action)
		}
	}
	sort.Strings(out)
	return out
}

func matches(pattern, value string) bool {
	pattern, value = strings.TrimSpace(pattern), strings.TrimSpace(value)
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func matchesResource(pattern, value string) bool {
	if strings.TrimSpace(pattern) == "" {
		return true
	}
	return matches(pattern, value)
}
