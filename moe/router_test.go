package moe

import (
	"strings"
	"testing"
	"unicode"
)

func TestRouterUsesTokenAndPhraseBoundaries(t *testing.T) {
	router := regressionRouter()
	for name, query := range map[string]string{
		"plan substring": "Give a concise explanation.",
		"api substring":  "Summarize this diagnostic.",
		"phrase":         "Rewrite this release-note clearly.",
	} {
		t.Run(name, func(t *testing.T) {
			selection := router.Route(query, RoutingContext{})
			if selection.Primary.ID != "general" {
				t.Fatalf("Route(%q) = %q, want general", query, selection.Primary.ID)
			}
		})
	}
	selection := router.Route("DEBUG—this API, then write a TEST.", RoutingContext{HasCodeContext: true})
	if selection.Primary.ID != "coding" {
		t.Fatalf("punctuated route = %q, want coding", selection.Primary.ID)
	}
}

func TestRouterSynthesizesOnlyDistinctDomainEvidence(t *testing.T) {
	router := regressionRouter()
	selection := router.Route("Plan an API refactor with tests, deployment, and rollback.", RoutingContext{})
	if selection.Primary.ID != "synthesis" || len(selection.Secondary) != 2 {
		t.Fatalf("cross-domain route = %+v", selection)
	}
	selection = router.Route("Plan the strategy.", RoutingContext{})
	if selection.Primary.ID != "synthesis" || len(selection.Secondary) != 0 {
		t.Fatalf("explicit synthesis route = %+v", selection)
	}
	selection = router.Route("Rewrite this deployment release note.", RoutingContext{})
	if selection.Primary.ID != "general" {
		t.Fatalf("general content route = %q, want general", selection.Primary.ID)
	}
}

func TestRouterFallsBackToGeneralOnLowConfidence(t *testing.T) {
	selection := regressionRouter().Route("Please handle this unfamiliar request.", RoutingContext{})
	if selection.Primary.ID != "general" || strings.Contains(selection.Reasoning, "synthesis") {
		t.Fatalf("fallback = %+v", selection)
	}
}

func FuzzRouterDoesNotMatchEmbeddedKeywords(f *testing.F) {
	f.Add("ex", "ation")
	f.Add("diag", "ostic")
	f.Fuzz(func(t *testing.T, prefix, suffix string) {
		letters := func(value string) string {
			var output strings.Builder
			for _, char := range value {
				if unicode.IsLetter(char) {
					output.WriteRune(char)
				}
			}
			return output.String()
		}
		query := "x" + letters(prefix) + "plan" + letters(suffix) + "y"
		selection := regressionRouter().Route(query, RoutingContext{})
		if selection.Primary.ID != "general" {
			t.Fatalf("embedded token %q routed to %q", query, selection.Primary.ID)
		}
	})
}

func BenchmarkRouterRoute(b *testing.B) {
	router := regressionRouter()
	query := "Create a strategy to compare evidence, refactor an API, and deploy with rollback."
	ctx := RoutingContext{HasCodeContext: true, HasBuildContext: true, Topics: []string{"production"}}
	b.ReportAllocs()
	for b.Loop() {
		_ = router.Route(query, ctx)
	}
}

func regressionRouter() *Router {
	registry := NewRegistry()
	registry.Register(&ExpertDefinition{ID: "general", Keywords: []string{"explain", "rewrite", "release note", "diagnostic"}, Priority: 20})
	registry.Register(&ExpertDefinition{ID: "coding", Keywords: []string{"code", "debug", "api", "test", "refactor", "review"}, Priority: 10})
	registry.Register(&ExpertDefinition{ID: "research", Keywords: []string{"research", "compare", "evidence", "source"}, Priority: 20})
	registry.Register(&ExpertDefinition{ID: "operations", Keywords: []string{"deploy", "deployment", "production", "rollback", "release"}, Priority: 20})
	registry.Register(&ExpertDefinition{ID: "synthesis", Keywords: []string{"plan", "strategy", "architecture"}, Priority: 100, CanSynthesize: true})
	return NewRouter(registry, DefaultRouterConfig())
}
