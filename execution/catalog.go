package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/Anurag607/amoeba/policy"
)

// ToolRef pins the exact admitted tool registration.
type ToolRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolClass describes side effects and trust independently of prompts.
type ToolClass struct {
	Category      string `json:"category"`
	SideEffecting bool   `json:"side_effecting"`
	HostAction    bool   `json:"host_action"`
	TrustedOutput bool   `json:"trusted_output"`
}

type Validator func(arguments string) error
type Canonicalizer func(arguments string) (string, error)
type ResourceResolver func(canonicalArguments string) (string, error)
type AuthorizationResolver func(canonicalArguments string) ([]AuthorizationTarget, error)
type ToolFunc func(context.Context, string) (string, error)

// ToolSpec is one immutable, executable catalog registration.
type ToolSpec struct {
	Ref          ToolRef
	Description  string
	Schema       json.RawMessage
	SchemaDigest string
	ToolSets     []string
	Class        ToolClass
	Validate     Validator
	// Canonicalize returns the exact JSON passed to execution and hashed into
	// approvals. When nil, the catalog uses deterministic JSON encoding.
	Canonicalize Canonicalizer
	// ResolveResource derives the policy resource from canonical arguments.
	// Resource-sensitive and side-effecting tools must provide it; callers are
	// never trusted to describe their own authorization target.
	ResolveResource ResourceResolver
	// ResolveAuthorizations adds argument-derived capabilities, such as the
	// selected expert or skill, that must also pass the admitted policy.
	ResolveAuthorizations AuthorizationResolver
	Execute               ToolFunc
}

// Catalog registers tools and produces immutable per-run snapshots.
type Catalog struct {
	mu    sync.RWMutex
	tools map[string]ToolSpec
}

func NewCatalog() *Catalog { return &Catalog{tools: make(map[string]ToolSpec)} }

func (c *Catalog) Register(spec ToolSpec) error {
	if strings.TrimSpace(spec.Ref.Name) == "" || strings.TrimSpace(spec.Ref.Version) == "" {
		return fmt.Errorf("tool name and version are required")
	}
	if strings.TrimSpace(spec.SchemaDigest) == "" {
		return fmt.Errorf("tool %s requires a schema digest", spec.Ref.Name)
	}
	if spec.Validate == nil || spec.Execute == nil {
		return fmt.Errorf("tool %s requires validator and executor", spec.Ref.Name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.tools[spec.Ref.Name]; ok && existing.Ref.Version == spec.Ref.Version {
		return fmt.Errorf("tool %s@%s already registered", spec.Ref.Name, spec.Ref.Version)
	}
	c.tools[spec.Ref.Name] = spec
	return nil
}

// CatalogSnapshot is an immutable admitted tool catalog.
type CatalogSnapshot struct {
	version string
	tools   map[string]ToolSpec
}

// Version returns the catalog generation pinned at admission.
func (s CatalogSnapshot) Version() string { return s.version }

func (c *Catalog) Snapshot(version string) (CatalogSnapshot, error) {
	if strings.TrimSpace(version) == "" {
		return CatalogSnapshot{}, fmt.Errorf("catalog snapshot version is required")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	tools := make(map[string]ToolSpec, len(c.tools))
	for name, spec := range c.tools {
		spec.Schema = append(json.RawMessage(nil), spec.Schema...)
		spec.ToolSets = append([]string(nil), spec.ToolSets...)
		tools[name] = spec
	}
	return CatalogSnapshot{version: version, tools: tools}, nil
}

type Capability struct {
	Ref          ToolRef         `json:"ref"`
	Description  string          `json:"description"`
	Schema       json.RawMessage `json:"schema"`
	SchemaDigest string          `json:"schema_digest"`
	Class        ToolClass       `json:"class"`
}

// Capabilities is the versioned source for model discovery. Consumers should
// generate provider-specific tool schemas from this same snapshot.
func (s CatalogSnapshot) Capabilities(toolSet string, snapshot policy.Snapshot) []Capability {
	out := make([]Capability, 0)
	for _, spec := range s.tools {
		if toolSet != "" && !contains(spec.ToolSets, toolSet) {
			continue
		}
		if snapshot.Decide(spec.Ref.Name, "").Effect == policy.EffectDeny {
			continue
		}
		out = append(out, Capability{Ref: spec.Ref, Description: spec.Description, Schema: append(json.RawMessage(nil), spec.Schema...), SchemaDigest: spec.SchemaDigest, Class: spec.Class})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.Name < out[j].Ref.Name })
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s CatalogSnapshot) Resolve(ref ToolRef) (ToolSpec, bool) {
	spec, ok := s.tools[ref.Name]
	return spec, ok && spec.Ref.Version == ref.Version
}

// CanonicalOperation validates a tool call and derives its immutable policy
// target. assertedResource is only a caller-side consistency check.
func (s CatalogSnapshot) CanonicalOperation(ref ToolRef, assertedResource, arguments string) (Operation, error) {
	spec, ok := s.Resolve(ref)
	if !ok {
		return Operation{}, fmt.Errorf("tool %s@%s is not in the admitted catalog", ref.Name, ref.Version)
	}
	if err := spec.Validate(arguments); err != nil {
		return Operation{}, fmt.Errorf("validate tool %s: %w", ref.Name, err)
	}
	var canonical string
	var err error
	if spec.Canonicalize != nil {
		canonical, err = spec.Canonicalize(arguments)
	} else {
		canonical, err = canonicalJSON(arguments)
	}
	if err != nil {
		return Operation{}, fmt.Errorf("canonicalize tool %s: %w", ref.Name, err)
	}
	resource := ""
	if spec.ResolveResource != nil {
		resource, err = spec.ResolveResource(canonical)
		if err != nil {
			return Operation{}, fmt.Errorf("resolve tool %s resource: %w", ref.Name, err)
		}
		resource = strings.TrimSpace(resource)
	} else if spec.Class.SideEffecting || spec.Class.HostAction || strings.TrimSpace(assertedResource) != "" {
		return Operation{}, fmt.Errorf("tool %s requires a resource resolver", ref.Name)
	}
	if asserted := strings.TrimSpace(assertedResource); asserted != "" && asserted != resource {
		return Operation{}, fmt.Errorf("tool %s asserted resource does not match canonical resource", ref.Name)
	}
	var authorizations []AuthorizationTarget
	if spec.ResolveAuthorizations != nil {
		authorizations, err = spec.ResolveAuthorizations(canonical)
		if err != nil {
			return Operation{}, fmt.Errorf("resolve tool %s authorizations: %w", ref.Name, err)
		}
		for _, target := range authorizations {
			if strings.TrimSpace(target.Action) == "" {
				return Operation{}, fmt.Errorf("tool %s returned an empty authorization action", ref.Name)
			}
		}
	}
	return newOperation(ref, s.version, spec.SchemaDigest, canonical, resource, authorizations), nil
}

func canonicalJSON(arguments string) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("multiple JSON values")
		}
		return "", err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (s CatalogSnapshot) Refs(policySnapshot policy.Snapshot) []ToolRef {
	refs := make([]ToolRef, 0, len(s.tools))
	for _, spec := range s.tools {
		if policySnapshot.Decide(spec.Ref.Name, "").Effect != policy.EffectDeny {
			refs = append(refs, spec.Ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs
}
