package evaluation

import "github.com/Anurag607/amoeba/runtimekit"

// RoutingSplit separates cases used while tuning from cases that guard
// against overfitting. Holdout expectations must not be changed to make an
// implementation pass.
type RoutingSplit string

const (
	RoutingDevelopment RoutingSplit = "development"
	RoutingHoldout     RoutingSplit = "holdout"
)

// RoutingCase is a provider-free router regression.
type RoutingCase struct {
	ID             string
	Split          RoutingSplit
	Query          string
	Routing        runtimekit.RoutingInput
	ExpectedExpert string
}

// RoutingCases returns the frozen development and holdout corpus. It is kept
// separate from BuiltInCases because it measures deterministic planner
// routing without spending model tokens.
func RoutingCases() []RoutingCase {
	return []RoutingCase{
		{ID: "dev-general-arithmetic", Split: RoutingDevelopment, Query: "Calculate 31 times 7.", ExpectedExpert: "general"},
		{ID: "dev-general-writing", Split: RoutingDevelopment, Query: "Rewrite this release note clearly.", ExpectedExpert: "general"},
		{ID: "dev-general-clarification", Split: RoutingDevelopment, Query: "Help clarify the request before answering.", ExpectedExpert: "general"},
		{ID: "dev-coding-debug", Split: RoutingDevelopment, Query: "Debug this code and add a test.", Routing: codeContext(), ExpectedExpert: "coding"},
		{ID: "dev-coding-api", Split: RoutingDevelopment, Query: "Review the API function and refactor it.", ExpectedExpert: "coding"},
		{ID: "dev-coding-build", Split: RoutingDevelopment, Query: "Fix the compile failure in the CI pipeline.", Routing: buildContext(), ExpectedExpert: "coding"},
		{ID: "dev-research-sources", Split: RoutingDevelopment, Query: "Compare these sources and weigh the evidence.", ExpectedExpert: "research"},
		{ID: "dev-research-study", Split: RoutingDevelopment, Query: "Analyze the study and verify its citation.", ExpectedExpert: "research"},
		{ID: "dev-research-analysis", Split: RoutingDevelopment, Query: "Research a comparison using supplied evidence.", ExpectedExpert: "research"},
		{ID: "dev-operations-deploy", Split: RoutingDevelopment, Query: "Deploy to production with a rollback.", Routing: buildContext(), ExpectedExpert: "operations"},
		{ID: "dev-operations-incident", Split: RoutingDevelopment, Query: "Inspect incident logs and latency telemetry.", ExpectedExpert: "operations"},
		{ID: "dev-operations-monitor", Split: RoutingDevelopment, Query: "Monitor the rollout for reliability regressions.", ExpectedExpert: "operations"},
		{ID: "dev-synthesis-code-ops", Split: RoutingDevelopment, Query: "Plan an API refactor with code tests, a production deployment, rollout, and rollback.", ExpectedExpert: "synthesis"},
		{ID: "dev-synthesis-research-ops", Split: RoutingDevelopment, Query: "Create a strategy that compares evidence before a production rollout.", ExpectedExpert: "synthesis"},
		{ID: "dev-synthesis-migration", Split: RoutingDevelopment, Query: "Coordinate a database migration with code tests, production deployment, rollout, and rollback.", ExpectedExpert: "synthesis"},

		{ID: "holdout-general-boundary-plan", Split: RoutingHoldout, Query: "Give a short explanation of the result.", ExpectedExpert: "general"},
		{ID: "holdout-general-boundary-api", Split: RoutingHoldout, Query: "Summarize this diagnostic for a nontechnical reader.", ExpectedExpert: "general"},
		{ID: "holdout-general-secret", Split: RoutingHoldout, Query: "Redact the credential and return only the safe value.", ExpectedExpert: "general"},
		{ID: "holdout-coding-punctuation", Split: RoutingHoldout, Query: "DEBUG—this API; then write a TEST.", Routing: codeContext(), ExpectedExpert: "coding"},
		{ID: "holdout-coding-review", Split: RoutingHoldout, Query: "Review this diff and commit the smallest bug fix.", Routing: codeContext(), ExpectedExpert: "coding"},
		{ID: "holdout-coding-database", Split: RoutingHoldout, Query: "Refactor the database migration function.", ExpectedExpert: "coding"},
		{ID: "holdout-research-plural", Split: RoutingHoldout, Query: "Compare the sources and citations for this study.", ExpectedExpert: "research"},
		{ID: "holdout-research-unicode", Split: RoutingHoldout, Query: "ANALYZE—this evidence, then compare it.", ExpectedExpert: "research"},
		{ID: "holdout-research-context", Split: RoutingHoldout, Query: "Assess the supplied source quality.", Routing: runtimekit.RoutingInput{Topics: []string{"research", "evidence"}}, ExpectedExpert: "research"},
		{ID: "holdout-operations-ambiguity", Split: RoutingHoldout, Query: "Deploy it, but ask for the target environment first.", ExpectedExpert: "operations"},
		{ID: "holdout-operations-observability", Split: RoutingHoldout, Query: "Trace the production latency incident.", Routing: runtimekit.RoutingInput{HasTraceContext: true}, ExpectedExpert: "operations"},
		{ID: "holdout-operations-release", Split: RoutingHoldout, Query: "Monitor the release rollout and prepare rollback.", ExpectedExpert: "operations"},
		{ID: "holdout-synthesis-code-ops", Split: RoutingHoldout, Query: "Plan code review gates and production rollback for the deployment.", ExpectedExpert: "synthesis"},
		{ID: "holdout-synthesis-research-code", Split: RoutingHoldout, Query: "Develop a strategy to compare evidence before refactoring the API.", ExpectedExpert: "synthesis"},
		{ID: "holdout-synthesis-three-domain", Split: RoutingHoldout, Query: "Coordinate research, API tests, and a monitored production rollout.", ExpectedExpert: "synthesis"},
	}
}

func codeContext() runtimekit.RoutingInput {
	return runtimekit.RoutingInput{HasCodeContext: true}
}

func buildContext() runtimekit.RoutingInput {
	return runtimekit.RoutingInput{HasBuildContext: true}
}
