package evaluation

import (
	"context"
	"testing"

	"github.com/anurgosw/agentic-moe/runtimekit"
)

func TestRoutingCorpusMatchesDefaultRuntime(t *testing.T) {
	runtime, err := runtimekit.New(runtimekit.DefaultConfig(), runtimekit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	counts := map[RoutingSplit]int{}
	seen := map[string]struct{}{}
	for _, item := range RoutingCases() {
		if _, duplicate := seen[item.ID]; duplicate {
			t.Fatalf("duplicate routing case %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		counts[item.Split]++
		plan, err := runtime.Plan(context.Background(), item.Query, item.Routing)
		if err != nil {
			t.Errorf("%s: Plan() error = %v", item.ID, err)
			continue
		}
		if plan.Selection.Primary != item.ExpectedExpert {
			t.Errorf("%s (%s): selected %q, want %q", item.ID, item.Split, plan.Selection.Primary, item.ExpectedExpert)
		}
	}
	if counts[RoutingDevelopment] < 15 || counts[RoutingHoldout] < 15 {
		t.Fatalf("routing corpus too small: %v", counts)
	}
}
