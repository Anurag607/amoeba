package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/Anurag607/amoeba/execution"
)

func TestEngineExecutesDAGAndSkipsFailedDependents(t *testing.T) {
	owner := execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}
	graph := Graph{ID: "g", Revision: 1, Nodes: []Node{
		{ID: "a", Kind: "ok", InputDigest: "a"},
		{ID: "b", Kind: "fail", InputDigest: "b", DependsOn: []string{"a"}},
		{ID: "c", Kind: "ok", InputDigest: "c", DependsOn: []string{"b"}},
	}}
	engine := Engine{Store: NewMemoryStore(), Handlers: map[string]Handler{
		"ok": HandlerFunc(func(context.Context, execution.Identity, Node) (string, string, error) {
			return "sha256:artifact", "", nil
		}),
		"fail": HandlerFunc(func(context.Context, execution.Identity, Node) (string, string, error) {
			return "", "test.failed", fmt.Errorf("failed")
		}),
	}}
	run, err := engine.Run(context.Background(), Run{ID: "run", Owner: owner, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	if run.Nodes["a"].State != NodeSucceeded || run.Nodes["b"].State != NodeFailed || run.Nodes["c"].State != NodeSkipped {
		t.Fatalf("nodes=%+v", run.Nodes)
	}
}

func TestGraphRejectsCycles(t *testing.T) {
	graph := Graph{ID: "g", Nodes: []Node{{ID: "a", Kind: "x", InputDigest: "a", DependsOn: []string{"b"}}, {ID: "b", Kind: "x", InputDigest: "b", DependsOn: []string{"a"}}}}
	if _, err := graph.TopologicalOrder(); err == nil {
		t.Fatal("cycle accepted")
	}
}
