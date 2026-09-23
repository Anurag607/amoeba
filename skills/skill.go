// Package skills holds the domain-knowledge primitives used by experts.
//
// A Skill is a Markdown body with a YAML frontmatter header describing its
// id, name, description, category, and priority. Skills may optionally
// include sibling "references" — named Markdown files the LLM can pull on
// demand via the load_reference tool.
//
// Skill content is typically embedded into the consumer's binary with Go's
// embed directive and loaded into a Registry at startup using LoadFS.
package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anurgosw/agentic-moe/policy"
)

// Skill is a single domain-knowledge module.
type Skill struct {
	ID            string
	Name          string
	Description   string
	Category      string
	Scope         string
	Source        string
	Trust         string
	Digest        string
	SchemaVersion int
	Precedence    int
	Priority      int
	// TokenEstimate is an optional author-supplied prompt cost. When zero, the
	// registry estimates cost from the rendered content.
	TokenEstimate int
	Content       string
	References    map[string]string
}

// ReferenceNames returns the sorted list of reference document names.
func (s *Skill) ReferenceNames() []string {
	if len(s.References) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.References))
	for k := range s.References {
		names = append(names, k)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// FormatForPrompt renders the skill as a labeled block for prompt injection.
func (s *Skill) FormatForPrompt() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== SKILL: %s ===\n", s.Name)
	sb.WriteString(s.Content)
	sb.WriteString("\n=== END SKILL ===\n")
	return sb.String()
}

// Registry holds registered skills.
type Registry struct {
	mu          sync.RWMutex
	skills      map[string]*Skill
	diagnostics []Diagnostic
	generation  uint64
	updatedAt   time.Time
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Skill)}
}

// Register adds a skill (overwriting any existing entry with the same ID).
func (r *Registry) Register(s *Skill) {
	if s == nil || strings.TrimSpace(s.ID) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s = cloneSkill(s)
	if s.Precedence == 0 {
		s.Precedence = ScopePrecedence(s.Scope)
	}
	if existing, ok := r.skills[s.ID]; ok && existing.Precedence > s.Precedence {
		return
	}
	r.skills[s.ID] = s
	r.generation++
	r.updatedAt = time.Now()
}

// ScopePrecedence derives deterministic override order. Explicit frontmatter
// precedence remains available for hosts with a more specific policy.
func ScopePrecedence(scope string) int {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "builtin":
		return 10
	case "global":
		return 20
	case "user":
		return 30
	case "workspace":
		return 40
	case "project":
		return 50
	case "session":
		return 60
	default:
		return 1
	}
}

// Get fetches a skill by ID.
func (r *Registry) Get(id string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[id]
	return cloneSkill(s), ok
}

// GetMultiple fetches multiple skills by ID. Missing IDs are silently skipped.
func (r *Registry) GetMultiple(ids []string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Skill, 0, len(ids))
	for _, id := range ids {
		if s, ok := r.skills[id]; ok {
			out = append(out, cloneSkill(s))
		}
	}
	return out
}

// GetByCategory returns all skills in a category.
func (r *Registry) GetByCategory(category string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Skill
	for _, s := range r.skills {
		if s.Category == category {
			out = append(out, cloneSkill(s))
		}
	}
	sortSkills(out)
	return out
}

// AllIDs returns every registered skill ID.
func (r *Registry) AllIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.skills))
	for id := range r.skills {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// All returns every registered skill, sorted by priority ascending.
func (r *Registry) All() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, cloneSkill(s))
	}
	sortSkills(out)
	return out
}

// Visible returns only skills allowed by the admitted policy snapshot.
func (r *Registry) Visible(snapshot policy.Snapshot) []*Skill {
	all := r.All()
	out := make([]*Skill, 0, len(all))
	for _, skill := range all {
		if snapshot.Decide("skill."+skill.ID, skill.Scope).Effect != policy.EffectDeny {
			out = append(out, skill)
		}
	}
	return out
}

type RegistryStatus struct {
	Generation  uint64       `json:"generation"`
	SkillCount  int          `json:"skill_count"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

func (r *Registry) Status() RegistryStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RegistryStatus{Generation: r.generation, SkillCount: len(r.skills), UpdatedAt: r.updatedAt, Diagnostics: append([]Diagnostic(nil), r.diagnostics...)}
}

func (r *Registry) addDiagnostic(d Diagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.diagnostics = append(r.diagnostics, d)
	r.generation++
	r.updatedAt = time.Now()
}

// GetReference returns a named reference document attached to a skill.
func (r *Registry) GetReference(skillID, refName string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[skillID]
	if !ok || s.References == nil {
		return "", false
	}
	content, ok := s.References[refName]
	return content, ok
}

// AssembleContext concatenates the formatted bodies of the named skills,
// sorted by priority, into a single prompt-ready block.
func (r *Registry) AssembleContext(skillIDs []string) string {
	context, _, _ := r.AssembleContextBudget(skillIDs, 0)
	return context
}

// AssembleContextBudget renders skills in deterministic priority/ID order up
// to maxTokens. A non-positive budget is unlimited. Loaded and omitted IDs
// let hosts expose an honest prompt manifest and offer omitted skills through
// load_skill on demand.
func (r *Registry) AssembleContextBudget(skillIDs []string, maxTokens int) (string, []string, []string) {
	skills := r.GetMultiple(skillIDs)
	if len(skills) == 0 {
		return "", nil, nil
	}
	sortSkills(skills)
	var sb strings.Builder
	const header = "\n\n## DOMAIN KNOWLEDGE (Skills)\n\nThe following domain knowledge has been loaded to assist your analysis. Use these strategies and patterns when investigating.\n\n"
	used := estimateTokens(header)
	loaded := make([]string, 0, len(skills))
	omitted := make([]string, 0)
	for _, s := range skills {
		rendered := s.FormatForPrompt() + "\n"
		cost := s.TokenEstimate
		if cost <= 0 {
			cost = estimateTokens(rendered)
		}
		if maxTokens > 0 && used+cost > maxTokens {
			omitted = append(omitted, s.ID)
			continue
		}
		if len(loaded) == 0 {
			sb.WriteString(header)
		}
		sb.WriteString(rendered)
		used += cost
		loaded = append(loaded, s.ID)
	}
	return sb.String(), loaded, omitted
}

func sortSkills(in []*Skill) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Priority != in[j].Priority {
			return in[i].Priority < in[j].Priority
		}
		return in[i].ID < in[j].ID
	})
}

func estimateTokens(s string) int {
	n := len([]rune(s)) / 4
	if n < 1 {
		return 1
	}
	return n
}

func cloneSkill(skill *Skill) *Skill {
	if skill == nil {
		return nil
	}
	out := *skill
	if skill.References != nil {
		out.References = make(map[string]string, len(skill.References))
		for name, content := range skill.References {
			out.References[name] = content
		}
	}
	return &out
}
