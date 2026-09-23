package contextengine

import (
	"context"
	"testing"
)

type testRetriever struct{ results []RetrievalResult }

func (testRetriever) Revision(context.Context, RetrievalQuery) (string, error) { return "index-1", nil }
func (r testRetriever) Search(context.Context, RetrievalQuery) ([]RetrievalResult, error) {
	return r.results, nil
}

func TestRetrievalSourceRejectsScopeEscapeAndSkipsTombstones(t *testing.T) {
	source := RetrievalSource{SourceKey: "memory.search", Query: RetrievalQuery{Text: "needle", Limit: 5, AccessScopes: []string{"workspace:a"}}, Retriever: testRetriever{results: []RetrievalResult{
		{DocumentID: "d1", ChunkID: "c1", Revision: "1", Provenance: "file:a", Content: "kept", AccessScope: "workspace:a"},
		{DocumentID: "d2", ChunkID: "c2", Revision: "1", Provenance: "file:b", Content: "deleted", AccessScope: "workspace:a", Tombstoned: true},
	}}}
	frames, err := source.Resolve(context.Background())
	if err != nil || len(frames) != 1 || frames[0].Trust != TrustUntrusted || frames[0].Class != ClassMemory {
		t.Fatalf("frames=%+v err=%v", frames, err)
	}
	source.Retriever = testRetriever{results: []RetrievalResult{{DocumentID: "d", ChunkID: "c", Revision: "1", Provenance: "file", AccessScope: "workspace:b"}}}
	if _, err := source.Resolve(context.Background()); err == nil {
		t.Fatal("retrieval scope escape accepted")
	}
}
