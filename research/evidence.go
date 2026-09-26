// Package research defines an optional, revisioned evidence system of record.
// Collection, browsing, storage encryption, and report presentation remain
// host responsibilities.
package research

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

type SourceSnapshot struct {
	ID          string    `json:"id"`
	Locator     string    `json:"locator"`
	Digest      string    `json:"digest"`
	ContentRef  string    `json:"content_ref"`
	CapturedAt  time.Time `json:"captured_at"`
	ContentType string    `json:"content_type,omitempty"`
}

type Evidence struct {
	ID               string  `json:"id"`
	SourceSnapshotID string  `json:"source_snapshot_id"`
	Locator          string  `json:"locator"`
	ExcerptHash      string  `json:"excerpt_hash"`
	Quality          float64 `json:"quality"`
}

type Claim struct {
	ID            string `json:"id"`
	StatementHash string `json:"statement_hash"`
	Status        string `json:"status"`
}

type Relation string

const (
	RelationSupports    Relation = "supports"
	RelationContradicts Relation = "contradicts"
)

type ClaimEvidence struct {
	ClaimID    string   `json:"claim_id"`
	EvidenceID string   `json:"evidence_id"`
	Relation   Relation `json:"relation"`
	Strength   float64  `json:"strength"`
}

type Graph struct {
	ID        string             `json:"id"`
	Owner     execution.Identity `json:"owner"`
	Sources   []SourceSnapshot   `json:"sources"`
	Evidence  []Evidence         `json:"evidence"`
	Claims    []Claim            `json:"claims"`
	Edges     []ClaimEvidence    `json:"edges"`
	Revision  uint64             `json:"revision"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type Assessment struct {
	UnsupportedClaims  []string `json:"unsupported_claims,omitempty"`
	ContradictedClaims []string `json:"contradicted_claims,omitempty"`
	Coverage           float64  `json:"coverage"`
}

// CitationVerifier can perform domain-specific locator and quotation checks
// against the immutable source snapshot referenced by an evidence record.
type CitationVerifier interface {
	Verify(context.Context, SourceSnapshot, Evidence) error
}

type Store interface {
	Create(context.Context, Graph) (Graph, error)
	CompareAndSwap(context.Context, execution.Identity, string, uint64, Graph) (Graph, error)
	Get(context.Context, execution.Identity, string) (Graph, bool, error)
}

func (g Graph) Validate() error {
	if g.ID == "" || g.Owner.Validate() != nil {
		return fmt.Errorf("research graph requires ID and owner")
	}
	sources := make(map[string]struct{}, len(g.Sources))
	for _, source := range g.Sources {
		if source.ID == "" || source.Locator == "" || !validDigest(source.Digest) || source.ContentRef == "" || source.CapturedAt.IsZero() {
			return fmt.Errorf("source snapshot requires ID, locator, digest, content reference, and capture time")
		}
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate source snapshot %q", source.ID)
		}
		sources[source.ID] = struct{}{}
	}
	evidence := make(map[string]struct{}, len(g.Evidence))
	for _, item := range g.Evidence {
		if item.ID == "" || item.Locator == "" || !validDigest(item.ExcerptHash) || item.Quality < 0 || item.Quality > 1 {
			return fmt.Errorf("evidence requires ID, locator, excerpt hash, and bounded quality")
		}
		if _, ok := sources[item.SourceSnapshotID]; !ok {
			return fmt.Errorf("evidence %q references unknown source snapshot", item.ID)
		}
		if _, duplicate := evidence[item.ID]; duplicate {
			return fmt.Errorf("duplicate evidence %q", item.ID)
		}
		evidence[item.ID] = struct{}{}
	}
	claims := make(map[string]struct{}, len(g.Claims))
	for _, claim := range g.Claims {
		if claim.ID == "" || !validDigest(claim.StatementHash) {
			return fmt.Errorf("claim requires ID and statement hash")
		}
		if _, duplicate := claims[claim.ID]; duplicate {
			return fmt.Errorf("duplicate claim %q", claim.ID)
		}
		claims[claim.ID] = struct{}{}
	}
	edges := make(map[string]struct{}, len(g.Edges))
	for _, edge := range g.Edges {
		if _, ok := claims[edge.ClaimID]; !ok {
			return fmt.Errorf("claim-evidence edge references unknown claim")
		}
		if _, ok := evidence[edge.EvidenceID]; !ok {
			return fmt.Errorf("claim-evidence edge references unknown evidence")
		}
		if edge.Relation != RelationSupports && edge.Relation != RelationContradicts || edge.Strength < 0 || edge.Strength > 1 {
			return fmt.Errorf("claim-evidence edge has invalid relation or strength")
		}
		key := edge.ClaimID + "\x00" + edge.EvidenceID + "\x00" + string(edge.Relation)
		if _, duplicate := edges[key]; duplicate {
			return fmt.Errorf("duplicate claim-evidence edge")
		}
		edges[key] = struct{}{}
	}
	return nil
}

func (g Graph) Assess(minStrength float64) Assessment {
	if minStrength <= 0 || minStrength > 1 {
		minStrength = 0.5
	}
	supported := make(map[string]bool, len(g.Claims))
	contradicted := make(map[string]bool, len(g.Claims))
	for _, edge := range g.Edges {
		if edge.Strength < minStrength {
			continue
		}
		if edge.Relation == RelationSupports {
			supported[edge.ClaimID] = true
		} else {
			contradicted[edge.ClaimID] = true
		}
	}
	assessment := Assessment{}
	for _, claim := range g.Claims {
		if !supported[claim.ID] {
			assessment.UnsupportedClaims = append(assessment.UnsupportedClaims, claim.ID)
		}
		if contradicted[claim.ID] {
			assessment.ContradictedClaims = append(assessment.ContradictedClaims, claim.ID)
		}
	}
	sort.Strings(assessment.UnsupportedClaims)
	sort.Strings(assessment.ContradictedClaims)
	if len(g.Claims) > 0 {
		assessment.Coverage = float64(len(g.Claims)-len(assessment.UnsupportedClaims)) / float64(len(g.Claims))
	}
	return assessment
}

func (g Graph) VerifyCitations(ctx context.Context, verifier CitationVerifier) error {
	if verifier == nil {
		return fmt.Errorf("citation verifier is required")
	}
	sources := make(map[string]SourceSnapshot, len(g.Sources))
	for _, source := range g.Sources {
		sources[source.ID] = source
	}
	for _, item := range g.Evidence {
		if err := verifier.Verify(ctx, sources[item.SourceSnapshotID], item); err != nil {
			return fmt.Errorf("verify evidence %s: %w", item.ID, err)
		}
	}
	return nil
}

type MemoryStore struct {
	mu     sync.Mutex
	graphs map[string]Graph
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{graphs: make(map[string]Graph)} }

func (s *MemoryStore) Create(ctx context.Context, graph Graph) (Graph, error) {
	if err := ctx.Err(); err != nil {
		return Graph{}, err
	}
	if err := graph.Validate(); err != nil {
		return Graph{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.graphs[graph.ID]; exists {
		return Graph{}, fmt.Errorf("research graph %q already exists", graph.ID)
	}
	graph.Revision, graph.UpdatedAt = 1, time.Now()
	s.graphs[graph.ID] = cloneGraph(graph)
	return cloneGraph(graph), nil
}

func (s *MemoryStore) CompareAndSwap(ctx context.Context, owner execution.Identity, id string, expected uint64, next Graph) (Graph, error) {
	if err := ctx.Err(); err != nil {
		return Graph{}, err
	}
	if next.ID != id || next.Owner != owner {
		return Graph{}, fmt.Errorf("research graph identity cannot change")
	}
	if err := next.Validate(); err != nil {
		return Graph{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.graphs[id]
	if !ok || current.Owner != owner || current.Revision != expected {
		return Graph{}, fmt.Errorf("research graph missing, owner mismatch, or revision conflict")
	}
	if err := snapshotsRemainImmutable(current.Sources, next.Sources); err != nil {
		return Graph{}, err
	}
	next.Revision, next.UpdatedAt = current.Revision+1, time.Now()
	s.graphs[id] = cloneGraph(next)
	return cloneGraph(next), nil
}

func (s *MemoryStore) Get(ctx context.Context, owner execution.Identity, id string) (Graph, bool, error) {
	if err := ctx.Err(); err != nil {
		return Graph{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	graph, ok := s.graphs[id]
	if !ok {
		return Graph{}, false, nil
	}
	if graph.Owner != owner {
		return Graph{}, false, fmt.Errorf("research graph owner mismatch")
	}
	return cloneGraph(graph), true, nil
}

func snapshotsRemainImmutable(before, after []SourceSnapshot) error {
	next := make(map[string]SourceSnapshot, len(after))
	for _, snapshot := range after {
		next[snapshot.ID] = snapshot
	}
	for _, snapshot := range before {
		candidate, ok := next[snapshot.ID]
		if !ok || candidate != snapshot {
			return fmt.Errorf("source snapshot %q is immutable", snapshot.ID)
		}
	}
	return nil
}

func cloneGraph(graph Graph) Graph {
	graph.Sources = append([]SourceSnapshot(nil), graph.Sources...)
	graph.Evidence = append([]Evidence(nil), graph.Evidence...)
	graph.Claims = append([]Claim(nil), graph.Claims...)
	graph.Edges = append([]ClaimEvidence(nil), graph.Edges...)
	return graph
}

func validDigest(value string) bool { return strings.HasPrefix(value, "sha256:") }
