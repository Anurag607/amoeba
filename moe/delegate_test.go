package moe

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anurgosw/agentic-moe/agent"
	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/policy"
)

func TestDelegateRequiresLeaseAndRejectsLegacyContext(t *testing.T) {
	base, reg := delegateFixture()
	called := false
	if err := RegisterDelegate(&base, reg, func(context.Context, DelegateRequest, *ExpertDefinition) (*DelegateResult, error) {
		called = true
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDelegate(t, &base, context.Background(), "missing-lease", `{"expert_id":"research","task":"check"}`); err == nil || !strings.Contains(err.Error(), "lease") {
		t.Fatalf("expected missing lease error, got %v", err)
	}
	if _, err := executeDelegate(t, &base, context.Background(), "legacy-context", `{"expert_id":"research","task":"check","context":"trusted now"}`); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected strict legacy context rejection, got %v", err)
	}
	if called {
		t.Fatal("callback ran for rejected requests")
	}
}

func TestDelegateReturnsValidatedArtifactAndLifecycleEvents(t *testing.T) {
	base, reg := delegateFixture()
	var events []Event
	if err := RegisterDelegateWithOptions(&base, reg, func(ctx context.Context, req DelegateRequest, _ *ExpertDefinition) (*DelegateResult, error) {
		if DelegationTokenGrantFromContext(ctx) == 0 {
			t.Fatal("child did not receive its token grant")
		}
		if len(req.ContextFrames) != 1 || req.ContextFrames[0].Content != "canonical data" {
			t.Fatalf("callback did not receive canonical context: %+v", req.ContextFrames)
		}
		return &DelegateResult{Artifact: DelegationArtifact{
			Summary: "verified", SourceRefs: []string{"file:a.go"}, Coverage: []string{"requested function"},
			Findings: []ArtifactFinding{{Claim: "safe", SourceRefs: []string{"file:a.go"}}},
		}, Usage: DelegationUsage{TotalTokens: 120}}, nil
	}, DelegateOptions{
		Emit: func(event Event) { events = append(events, event) },
		ResolveContext: func(_ context.Context, ref string) (ContextFragment, error) {
			return ContextFragment{Ref: ref, Content: "canonical data"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	lease := NewDelegationLease("run-1", DefaultDelegationLimits())
	ctx := WithDelegationLease(context.Background(), lease)
	out, err := executeDelegate(t, &base, ctx, "validated", `{"expert_id":"research","task":"check","selected_context":["file:a.go"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var outcome DelegationOutcome
	if err := json.Unmarshal([]byte(unwrappedToolOutput(out)), &outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Kind != "artifact" || outcome.Artifact == nil || outcome.Artifact.NodeID == "" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if len(events) != 2 || events[0].Type != EventTypeDelegationStarted || events[1].Type != EventTypeDelegationCompleted {
		t.Fatalf("unexpected lifecycle events: %+v", events)
	}
}

func TestDelegateRejectsUnselectedArtifactProvenance(t *testing.T) {
	base, reg := delegateFixture()
	if err := RegisterDelegateWithOptions(&base, reg, func(context.Context, DelegateRequest, *ExpertDefinition) (*DelegateResult, error) {
		return &DelegateResult{Artifact: DelegationArtifact{
			Summary: "unsupported", SourceRefs: []string{"web:other"}, Coverage: []string{"task"},
		}}, nil
	}, DelegateOptions{ResolveContext: func(_ context.Context, ref string) (ContextFragment, error) {
		return ContextFragment{Ref: ref, Content: "canonical data"}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	ctx := WithDelegationLease(context.Background(), NewDelegationLease("run-2", DefaultDelegationLimits()))
	out, err := executeDelegate(t, &base, ctx, "bad-provenance", `{"expert_id":"research","task":"check","selected_context":["file:a.go"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var outcome DelegationOutcome
	if err := json.Unmarshal([]byte(unwrappedToolOutput(out)), &outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Pressure == nil || outcome.Pressure.Code != "artifact_untrusted" {
		t.Fatalf("expected artifact_untrusted pressure, got %+v", outcome)
	}
}

func TestDelegateAcceptsOnlyCanonicalHostResolvedContext(t *testing.T) {
	base, reg := delegateFixture()
	called := false
	if err := RegisterDelegateWithOptions(&base, reg, func(context.Context, DelegateRequest, *ExpertDefinition) (*DelegateResult, error) {
		called = true
		return nil, nil
	}, DelegateOptions{ResolveContext: func(context.Context, string) (ContextFragment, error) {
		return ContextFragment{Ref: "forged:other", Content: "model-controlled"}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	ctx := WithDelegationLease(context.Background(), NewDelegationLease("run-context", DefaultDelegationLimits()))
	if _, err := executeDelegate(t, &base, ctx, "bad-context", `{"expert_id":"research","task":"check","selected_context":["file:a.go"]}`); err == nil || !strings.Contains(err.Error(), "mismatched") {
		t.Fatalf("expected canonical reference rejection, got %v", err)
	}
	if called {
		t.Fatal("callback ran with noncanonical context")
	}
}

func TestLeaseSnapshotPreservesDuplicateAndTokenBudgets(t *testing.T) {
	limits := DefaultDelegationLimits()
	limits.MaxTotalTokens = 100
	limits.MaxNodeTokens = 60
	lease := NewDelegationLease("run-resume", limits)
	permit, issue := lease.begin(context.Background(), "research", "same", 1, nil, false)
	if issue != nil {
		t.Fatal(issue)
	}
	artifact, issue := permit.finish(&DelegateResult{Artifact: DelegationArtifact{Summary: "ok", Coverage: []string{"task"}}, Usage: DelegationUsage{TotalTokens: 60}})
	if issue != nil {
		t.Fatal(issue)
	}
	artifact.Summary = "mutated by caller"
	snapshot, err := lease.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range snapshot.Artifacts {
		if retained.Summary != "ok" {
			t.Fatalf("retained artifact was not cloned: %q", retained.Summary)
		}
	}
	restored, err := RestoreDelegationLease(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, issue = restored.begin(context.Background(), "research", "same", 1, nil, false); issue == nil || issue.Code != "duplicate" {
		t.Fatalf("expected duplicate pressure after resume, got %+v", issue)
	}
	permit, issue = restored.begin(context.Background(), "research", "new", 1, nil, false)
	if issue != nil {
		t.Fatal(issue)
	}
	if permit.grant != 28 {
		t.Fatalf("grant reset across resume: got %d want 28", permit.grant)
	}
}

func TestLeaseReservesRootFinalizationAndBoundsReasoningWork(t *testing.T) {
	limits := DefaultDelegationLimits()
	limits.MaxTotalTokens = 100
	limits.RootFinalizationTokens = 20
	limits.MaxNodeTokens = 100
	limits.MaxWorkUnits = 2
	limits.MaxReasoningChildren = 1
	lease := NewDelegationLease("run-reasoning", limits)
	permit, issue := lease.begin(context.Background(), "research", "reason-1", 513, nil, true)
	if issue != nil {
		t.Fatal(issue)
	}
	if permit.grant != 80 || lease.RootFinalizationReserve() != 20 {
		t.Fatalf("reserve not enforced: grant=%d reserve=%d", permit.grant, lease.RootFinalizationReserve())
	}
	_, _ = permit.finish(&DelegateResult{Artifact: DelegationArtifact{Summary: "ok", Coverage: []string{"reasoning"}}, Usage: DelegationUsage{TotalTokens: 10}})
	if _, issue = lease.begin(context.Background(), "research", "reason-2", 1, nil, true); issue == nil || issue.Code != "reasoning_children" {
		t.Fatalf("expected reasoning child pressure, got %+v", issue)
	}
	if _, issue = lease.begin(context.Background(), "research", "task-2", 1, nil, false); issue == nil || issue.Code != "work_units" {
		t.Fatalf("expected work unit pressure, got %+v", issue)
	}
}

func TestFailedDelegateChargesFullGrantAndRetainsNoArtifact(t *testing.T) {
	base, reg := delegateFixture()
	if err := RegisterDelegate(&base, reg, func(context.Context, DelegateRequest, *ExpertDefinition) (*DelegateResult, error) {
		return &DelegateResult{Artifact: DelegationArtifact{Summary: "must not retain", Coverage: []string{"task"}}, Usage: DelegationUsage{TotalTokens: 1}}, errors.New("provider failed")
	}); err != nil {
		t.Fatal(err)
	}
	limits := DefaultDelegationLimits()
	limits.MaxTotalTokens = 50
	limits.MaxNodeTokens = 50
	lease := NewDelegationLease("run-failed", limits)
	ctx := WithDelegationLease(context.Background(), lease)
	if _, err := executeDelegate(t, &base, ctx, "failed", `{"expert_id":"research","task":"check"}`); err == nil {
		t.Fatal("expected delegated provider failure")
	}
	if _, issue := lease.begin(ctx, "research", "different", 1, nil, false); issue == nil || issue.Code != "tokens" {
		t.Fatalf("opaque failure did not consume full grant: %+v", issue)
	}
}

func TestLeaseDeadlineCancelsChildContext(t *testing.T) {
	limits := DefaultDelegationLimits()
	limits.Timeout = 10 * time.Millisecond
	lease := NewDelegationLease("run-deadline", limits)
	permit, issue := lease.begin(context.Background(), "research", "deadline", 1, nil, false)
	if issue != nil {
		t.Fatal(issue)
	}
	ctx := permit.context(context.Background())
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("child context did not inherit lease deadline")
	}
	_, _ = permit.finish(nil)
}

func TestLeaseBoundsParallelismAndDepth(t *testing.T) {
	limits := DefaultDelegationLimits()
	limits.MaxParallel = 1
	limits.MaxDepth = 1
	lease := NewDelegationLease("run-bounds", limits)
	first, issue := lease.begin(context.Background(), "research", "first", 1, nil, false)
	if issue != nil {
		t.Fatal(issue)
	}
	if _, issue = lease.begin(context.Background(), "research", "parallel", 1, nil, false); issue == nil || issue.Code != "parallel" {
		t.Fatalf("expected parallel pressure, got %+v", issue)
	}
	childCtx := first.context(context.Background())
	if _, issue = lease.begin(childCtx, "research", "nested", 1, nil, false); issue == nil || issue.Code != "depth" {
		t.Fatalf("expected depth pressure, got %+v", issue)
	}
	_, _ = first.finish(&DelegateResult{Artifact: DelegationArtifact{Summary: "ok", Coverage: []string{"task"}}})
}

func TestNestedDelegationYieldsParentParallelPermit(t *testing.T) {
	limits := DefaultDelegationLimits()
	limits.MaxParallel = 1
	limits.MaxDepth = 3
	lease := NewDelegationLease("run-yield", limits)
	root, issue := lease.begin(context.Background(), "research", "root", 1, nil, false)
	if issue != nil {
		t.Fatal(issue)
	}
	rootCtx := root.context(context.Background())
	child, issue := lease.begin(rootCtx, "research", "child", 1, nil, false)
	if issue != nil {
		t.Fatalf("parent did not yield: %+v", issue)
	}
	grandchild, issue := lease.begin(child.context(rootCtx), "research", "grandchild", 1, nil, false)
	if issue != nil {
		t.Fatalf("child did not yield: %+v", issue)
	}
	_, _ = grandchild.finish(&DelegateResult{Artifact: DelegationArtifact{Summary: "grandchild", Coverage: []string{"task"}}})
	_, _ = child.finish(&DelegateResult{Artifact: DelegationArtifact{Summary: "child", Coverage: []string{"task"}}})
	_, _ = root.finish(&DelegateResult{Artifact: DelegationArtifact{Summary: "root", Coverage: []string{"task"}}})
	if _, safe, err := lease.SnapshotIfSafe(); err != nil || !safe {
		t.Fatalf("lease did not return to safe boundary: safe=%v err=%v", safe, err)
	}
}

func delegateFixture() (agent.BaseAgent, *Registry) {
	base := agent.NewBase("central")
	reg := NewRegistry()
	reg.Register(&ExpertDefinition{ID: "research", Name: "Research"})
	return base, reg
}

func executeDelegate(t *testing.T, base *agent.BaseAgent, ctx context.Context, attemptID, arguments string) (string, error) {
	t.Helper()
	snapshot, err := policy.NewSnapshot("policy-v1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{
		{Action: "delegate_to_expert", Effect: policy.EffectAllow, ReasonCode: "delegate.allowed"},
		{Action: "expert.*", Effect: policy.EffectAllow, ReasonCode: "expert.allowed"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := base.Seal(agent.Admission{CatalogVersion: "catalog-v1", Policy: snapshot, Ledger: execution.NewMemoryAttemptLedger()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := execution.IdentityFromContext(ctx); !ok {
		ctx, err = execution.WithIdentity(ctx, execution.Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"})
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, err = agent.WithToolInvocation(ctx, agent.ToolInvocation{AttemptID: attemptID, RequestID: attemptID, IdempotencyKey: attemptID})
	if err != nil {
		t.Fatal(err)
	}
	return admitted.ExecuteTool(ctx, "delegate_to_expert", arguments)
}

func unwrappedToolOutput(out string) string {
	start := strings.Index(out, "<<<UNTRUSTED_TOOL_OUTPUT")
	if start < 0 {
		return out
	}
	start = strings.Index(out[start:], "\n") + start + 1
	end := strings.Index(out[start:], "\n<<<END_UNTRUSTED_TOOL_OUTPUT>>>")
	if end < 0 {
		return out
	}
	return out[start : start+end]
}
