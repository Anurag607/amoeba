package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Anurag607/amoeba/policy"
)

func TestCatalogDiscoverPagesWithoutEagerSchemas(t *testing.T) {
	catalog := NewCatalog()
	for _, name := range []string{"read_file", "search_code", "write_file"} {
		err := catalog.Register(ToolSpec{
			Ref: ToolRef{Name: name, Version: "1"}, Description: name, Schema: json.RawMessage(`{"type":"object"}`), SchemaDigest: "sha256:" + name,
			Validate: func(string) error { return nil }, Execute: func(context.Context, string) (string, error) { return "", nil },
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := catalog.Snapshot("v1")
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := policy.NewSnapshot("p1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{{Action: "*", Effect: policy.EffectAllow, ReasonCode: "test.allow"}}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := snapshot.Discover(CapabilityQuery{Search: "file", Limit: 1}, allowed)
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Schema) != 0 || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	next, err := snapshot.Discover(CapabilityQuery{Search: "file", Limit: 1, Cursor: page.NextCursor, IncludeSchemas: true}, allowed)
	if err != nil || len(next.Items) != 1 || len(next.Items[0].Schema) == 0 {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	other, _ := catalog.Snapshot("v2")
	if _, err := other.Discover(CapabilityQuery{Cursor: page.NextCursor}, allowed); err == nil {
		t.Fatal("cross-generation cursor accepted")
	}
}
