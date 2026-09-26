package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Anurag607/amoeba/policy"
)

func staticResource(resource string) ResourceResolver {
	return func(string) (string, error) { return resource, nil }
}

func admittedContext(t *testing.T) (context.Context, Identity) {
	t.Helper()
	identity := Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	ctx, err := WithIdentity(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, identity
}

func allowPolicy(t *testing.T, action string, effect policy.Effect) policy.Snapshot {
	t.Helper()
	snapshot, err := policy.NewSnapshot("policy-v1", policy.RuleSet{Layer: policy.LayerGlobal,
		Rules: []policy.Rule{{Action: action, Effect: effect, ReasonCode: action + ".decision"}}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestDispatcherCanonicalizesBeforePolicyAndWrapsOutput(t *testing.T) {
	catalog := NewCatalog()
	calledWith := ""
	err := catalog.Register(ToolSpec{Ref: ToolRef{Name: "read_file", Version: "v1"}, SchemaDigest: "sha256:read-v1",
		Validate: func(string) error { return nil }, ResolveResource: staticResource("file:a"),
		Execute: func(_ context.Context, arguments string) (string, error) {
			calledWith = arguments
			return "external", nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := catalog.Snapshot("catalog-v1")
	ctx, identity := admittedContext(t)
	ledger := NewMemoryAttemptLedger()
	dispatcher := Dispatcher{Catalog: snapshot, Policy: allowPolicy(t, "read_file", policy.EffectAllow), Ledger: ledger}
	out, err := dispatcher.Execute(ctx, "call-1", ToolRef{Name: "read_file", Version: "v1"}, "file:a", `{ "b": 2, "a": 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	if calledWith != `{"a":1,"b":2}` || !strings.Contains(out, "UNTRUSTED_TOOL_OUTPUT") {
		t.Fatalf("canonical=%q output=%q", calledWith, out)
	}
	attempt, ok, err := ledger.Get(ctx, identity, "call-1")
	if err != nil || !ok || attempt.State != AttemptSucceeded || attempt.TargetHash == "" || attempt.ResultHash == "" {
		t.Fatalf("unexpected attempt: %+v err=%v", attempt, err)
	}
}

type testApprover func(context.Context, ApprovalRequest) (ApprovalDecision, error)

func (f testApprover) Approve(ctx context.Context, request ApprovalRequest) (ApprovalDecision, error) {
	return f(ctx, request)
}

func TestDispatcherApprovalBindsCanonicalEnvelope(t *testing.T) {
	catalog := NewCatalog()
	_ = catalog.Register(ToolSpec{Ref: ToolRef{Name: "write_file", Version: "v1"}, SchemaDigest: "sha256:write-v1",
		Class: ToolClass{SideEffecting: true}, Validate: func(string) error { return nil }, ResolveResource: func(arguments string) (string, error) {
			var value struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(arguments), &value); err != nil {
				return "", err
			}
			return "file:" + value.Path, nil
		}, Execute: func(context.Context, string) (string, error) { return "ok", nil }})
	snapshot, _ := catalog.Snapshot("catalog-v1")
	ctx, identity := admittedContext(t)
	ledger, approvals := NewMemoryAttemptLedger(), NewMemoryApprovalStore()
	dispatcher := Dispatcher{Catalog: snapshot, Policy: allowPolicy(t, "write_file", policy.EffectApprove), Ledger: ledger, Approvals: approvals}
	ref := ToolRef{Name: "write_file", Version: "v1"}
	if _, err := dispatcher.Execute(ctx, "call-approval", ref, "file:a", `{"path":"a"}`); err == nil {
		t.Fatal("approval did not pause")
	}
	record, ok, err := approvals.Get(ctx, identity, "call-approval:approval")
	if err != nil || !ok {
		t.Fatalf("approval missing: %v", err)
	}
	if record.Request.Envelope.ArgsHash == "" || record.Request.Envelope.TargetHash == "" || record.Request.Envelope.CanonicalArguments != `{"path":"a"}` {
		t.Fatalf("unbound approval: %+v", record.Request)
	}
	_, err = approvals.Resolve(ctx, identity, record.Request.ID, record.Revision, ApprovalDecision{
		RequestID: record.Request.ID, EnvelopeHash: "tampered", Allowed: true,
	})
	if err == nil {
		t.Fatal("tampered approval was accepted")
	}
	_, err = approvals.Resolve(ctx, identity, record.Request.ID, record.Revision, ApprovalDecision{
		RequestID: record.Request.ID, EnvelopeHash: record.Request.EnvelopeHash, Allowed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.Execute(ctx, "call-approval", ref, "file:a", `{ "path": "a" }`); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherRejectsCallerResourceAndArgumentTampering(t *testing.T) {
	catalog := NewCatalog()
	_ = catalog.Register(ToolSpec{Ref: ToolRef{Name: "write", Version: "v1"}, SchemaDigest: "sha256:write-v1",
		Class: ToolClass{SideEffecting: true}, Validate: func(string) error { return nil },
		ResolveResource: staticResource("file:actual"), Execute: func(context.Context, string) (string, error) { return "", nil }})
	snapshot, _ := catalog.Snapshot("catalog-v1")
	ctx, _ := admittedContext(t)
	dispatcher := Dispatcher{Catalog: snapshot, Policy: allowPolicy(t, "write", policy.EffectAllow), Ledger: NewMemoryAttemptLedger()}
	if _, err := dispatcher.Execute(ctx, "call", ToolRef{Name: "write", Version: "v1"}, "file:benign", `{}`); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("caller-selected policy target accepted: %v", err)
	}
	if _, err := dispatcher.Execute(ctx, "call", ToolRef{Name: "write", Version: "v1"}, "file:actual", `{"changed":true}`); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.Execute(ctx, "call", ToolRef{Name: "write", Version: "v1"}, "file:actual", `{"changed":false}`); !errors.Is(err, ErrAttemptConflict) {
		t.Fatalf("argument-changing retry was accepted: %v", err)
	}
}

func TestSideEffectingFailureRequiresReconciliation(t *testing.T) {
	catalog := NewCatalog()
	_ = catalog.Register(ToolSpec{Ref: ToolRef{Name: "publish", Version: "v1"}, SchemaDigest: "sha256:publish-v1",
		Class: ToolClass{SideEffecting: true}, Validate: func(string) error { return nil }, ResolveResource: staticResource("topic:a"),
		Execute: func(context.Context, string) (string, error) { return "", errors.New("connection lost after send") }})
	snapshot, _ := catalog.Snapshot("catalog-v1")
	ctx, identity := admittedContext(t)
	ledger := NewMemoryAttemptLedger()
	dispatcher := Dispatcher{Catalog: snapshot, Policy: allowPolicy(t, "publish", policy.EffectAllow), Ledger: ledger}
	_, _ = dispatcher.Execute(ctx, "call-publish", ToolRef{Name: "publish", Version: "v1"}, "topic:a", `{}`)
	attempt, _, _ := ledger.Get(ctx, identity, "call-publish")
	if attempt.State != AttemptIndeterminate || attempt.ReconcileAction == "" {
		t.Fatalf("attempt=%+v", attempt)
	}
	items, err := ledger.ListForReconciliation(ctx, identity, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("reconciliation list=%v err=%v", items, err)
	}
}

func TestDispatcherRejectsExpiredApproval(t *testing.T) {
	catalog := NewCatalog()
	_ = catalog.Register(ToolSpec{Ref: ToolRef{Name: "write", Version: "v1"}, SchemaDigest: "sha256:write-v1",
		Class: ToolClass{SideEffecting: true}, Validate: func(string) error { return nil }, ResolveResource: staticResource("file:a"),
		Execute: func(context.Context, string) (string, error) { t.Fatal("executed"); return "", nil }})
	snapshot, _ := catalog.Snapshot("catalog-v1")
	ctx, identity := admittedContext(t)
	now := time.Unix(100, 0)
	ledger := NewMemoryAttemptLedger()
	dispatcher := Dispatcher{Catalog: snapshot, Policy: allowPolicy(t, "write", policy.EffectApprove), Ledger: ledger,
		Approver: testApprover(func(_ context.Context, request ApprovalRequest) (ApprovalDecision, error) {
			return ApprovalDecision{RequestID: request.ID, EnvelopeHash: request.EnvelopeHash, Allowed: true, ExpiresAt: now}, nil
		}), Now: func() time.Time { return now }}
	_, err := dispatcher.Execute(ctx, "expired", ToolRef{Name: "write", Version: "v1"}, "file:a", `{}`)
	if err == nil {
		t.Fatal("expired approval accepted")
	}
	attempt, _, _ := ledger.Get(ctx, identity, "expired")
	if attempt.State != AttemptDenied {
		t.Fatalf("attempt=%+v", attempt)
	}
}

func TestIdentityCannotBeRebound(t *testing.T) {
	first := Identity{PrincipalID: "u1", SessionID: "s1", RunID: "r1"}
	ctx, err := WithIdentity(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WithIdentity(ctx, Identity{PrincipalID: "u2", SessionID: "s1", RunID: "r1"}); err == nil {
		t.Fatal("identity rebind unexpectedly succeeded")
	}
}
