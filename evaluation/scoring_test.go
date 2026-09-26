package evaluation

import (
	"strings"
	"testing"

	"github.com/Anurag607/amoeba/runtimekit"
)

func TestBuiltInCasesAreValidAndFilterable(t *testing.T) {
	cases := BuiltInCases()
	if err := validateCases(cases); err != nil {
		t.Fatal(err)
	}
	selected, err := FilterCases(cases, []string{"coding, safety"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 4 {
		t.Fatalf("selected cases = %d, want 4", len(selected))
	}
	if _, err := FilterCases(cases, []string{"missing"}); err == nil {
		t.Fatal("missing domain error = nil")
	}
}

func TestBuiltInExpectedExpertsExist(t *testing.T) {
	planner, err := runtimekit.New(runtimekit.DefaultConfig(), runtimekit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = planner.Close() }()
	experts := make(map[string]struct{})
	for _, expert := range planner.Manifest().Experts {
		experts[expert.ID] = struct{}{}
	}
	for _, item := range BuiltInCases() {
		if _, ok := experts[item.ExpectedExpert]; !ok {
			t.Errorf("case %s expects unknown expert %s", item.ID, item.ExpectedExpert)
		}
	}
}

func TestScoreAnswerSupportsDeterministicChecks(t *testing.T) {
	item := Case{ID: "case", Domain: "test", Prompt: "prompt", Checks: []Check{
		{Name: "all", Kind: CheckContainsAll, Values: []string{"alpha", "beta"}},
		{Name: "any", Kind: CheckContainsAny, Values: []string{"gamma", "beta"}},
		{Name: "none", Kind: CheckExcludesAll, Values: []string{"secret"}},
		{Name: "words", Kind: CheckMaxWords, Values: []string{"3"}},
	}}
	score, assertions := scoreAnswer(item, "Alpha beta?")
	if score != 1 || len(assertions) != 4 {
		t.Fatalf("score = %v, assertions = %+v", score, assertions)
	}
	if !checkAnswer(Check{Kind: CheckQuestion}, "why?") {
		t.Fatal("question check failed")
	}
	if !checkAnswer(Check{Kind: CheckJSONKeys, Values: []string{"a"}}, `{"a":"b"}`) {
		t.Fatal("JSON check failed")
	}
	failedScore, failed := scoreAnswer(Case{Checks: []Check{
		{Name: "shape", Kind: CheckJSONKeys, Category: FailureFormat, Values: []string{"a"}},
		{Name: "safe", Kind: CheckExact, Category: FailureSafety, Values: []string{"SAFE"}},
	}}, "wrong")
	classes := failedClasses(failed)
	if failedScore != 0 || len(classes) != 2 || classes[0] != FailureSafety || classes[1] != FailureFormat {
		t.Fatalf("failure classification = %v, %v", failedScore, classes)
	}
}

func TestOutputMetadataBoundsAndRedacts(t *testing.T) {
	digest, preview := outputMetadata("api_key=sk-example-12345678 "+strings.Repeat("x", 100), 24)
	if len(digest) != 64 || strings.Contains(preview, "sk-example") || len([]rune(preview)) > 25 {
		t.Fatalf("digest = %q, preview = %q", digest, preview)
	}
}

func TestValidateCasesRejectsMalformedRules(t *testing.T) {
	for _, item := range []Case{
		{},
		{ID: "x", Domain: "d", Prompt: "p"},
		{ID: "x", Domain: "d", Prompt: "p", Checks: []Check{{Name: "bad", Kind: CheckRegexp, Pattern: "["}}},
	} {
		if err := validateCases([]Case{item}); err == nil {
			t.Fatalf("validateCases(%+v) error = nil", item)
		}
	}
}
