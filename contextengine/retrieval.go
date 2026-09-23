package contextengine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RetrievalQuery is host-authored search input. Access scopes are authority,
// not model-provided filters, and must be enforced again by the retriever.
type RetrievalQuery struct {
	Text         string   `json:"text"`
	Limit        int      `json:"limit"`
	AccessScopes []string `json:"access_scopes"`
}

type RetrievalResult struct {
	DocumentID  string    `json:"document_id"`
	ChunkID     string    `json:"chunk_id"`
	Revision    string    `json:"revision"`
	Provenance  string    `json:"provenance"`
	Content     string    `json:"content"`
	Score       float64   `json:"score"`
	FreshAt     time.Time `json:"fresh_at"`
	AccessScope string    `json:"access_scope"`
	Tombstoned  bool      `json:"tombstoned,omitempty"`
}

// Retriever supplies a stable revision and provenance-bearing results. The
// host owns indexing, deletion propagation, and authorization enforcement.
type Retriever interface {
	Revision(context.Context, RetrievalQuery) (string, error)
	Search(context.Context, RetrievalQuery) ([]RetrievalResult, error)
}

// RetrievalSource adapts a retriever into the ordinary context lifecycle so
// search results receive the same revision, spill, and reconciliation rules.
type RetrievalSource struct {
	SourceKey string
	Query     RetrievalQuery
	Retriever Retriever
}

func (s RetrievalSource) Key() string { return s.SourceKey }

func (s RetrievalSource) Revision(ctx context.Context) (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}
	return s.Retriever.Revision(ctx, s.Query)
}

func (s RetrievalSource) Resolve(ctx context.Context) ([]Frame, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	results, err := s.Retriever.Search(ctx, s.Query)
	if err != nil {
		return nil, err
	}
	frames := make([]Frame, 0, len(results))
	for _, result := range results {
		if result.Tombstoned {
			continue
		}
		if strings.TrimSpace(result.DocumentID) == "" || strings.TrimSpace(result.ChunkID) == "" || strings.TrimSpace(result.Revision) == "" || strings.TrimSpace(result.Provenance) == "" || strings.TrimSpace(result.AccessScope) == "" {
			return nil, fmt.Errorf("retrieval result requires document, chunk, revision, provenance, and access scope")
		}
		if !containsScope(s.Query.AccessScopes, result.AccessScope) {
			return nil, fmt.Errorf("retrieval result %s/%s escaped admitted access scopes", result.DocumentID, result.ChunkID)
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("encode retrieval result: %w", err)
		}
		frames = append(frames, Frame{Class: ClassMemory, Trust: TrustUntrusted, Content: string(payload)})
	}
	return frames, nil
}

func (s RetrievalSource) validate() error {
	if strings.TrimSpace(s.SourceKey) == "" || s.Retriever == nil || strings.TrimSpace(s.Query.Text) == "" || s.Query.Limit <= 0 || len(s.Query.AccessScopes) == 0 {
		return fmt.Errorf("retrieval source requires key, retriever, query, positive limit, and access scopes")
	}
	return nil
}

func containsScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}
