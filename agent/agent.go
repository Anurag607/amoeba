// Package agent composes tool registrations and seals them into the same
// execution catalog used for model discovery and dispatch.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/policy"
)

type ToolAgent interface {
	ExecuteTool(context.Context, string, string) (string, error)
	ToolNames() []string
	HasTool(string) bool
	ToolCount() int
	AgentType() string
}

// BaseAgent is a registration builder. It deliberately cannot execute tools;
// Seal produces an AdmittedAgent pinned to policy and catalog snapshots.
type BaseAgent struct {
	catalog   *execution.Catalog
	refs      map[string]execution.ToolRef
	toolOrder []string
	agentType string
}

func NewBase(agentType string) BaseAgent {
	return BaseAgent{catalog: execution.NewCatalog(), refs: make(map[string]execution.ToolRef), agentType: agentType}
}

func (a *BaseAgent) Register(spec execution.ToolSpec) error {
	if err := a.catalog.Register(spec); err != nil {
		return err
	}
	if _, exists := a.refs[spec.Ref.Name]; !exists {
		a.toolOrder = append(a.toolOrder, spec.Ref.Name)
	}
	a.refs[spec.Ref.Name] = spec.Ref
	return nil
}

type Admission struct {
	CatalogVersion string
	Policy         policy.Snapshot
	Ledger         execution.AttemptLedger
	Approver       execution.Approver
	Approvals      execution.ApprovalStore
}

func (a *BaseAgent) Seal(admission Admission) (*AdmittedAgent, error) {
	if admission.Ledger == nil {
		return nil, fmt.Errorf("agent admission requires an attempt ledger")
	}
	snapshot, err := a.catalog.Snapshot(admission.CatalogVersion)
	if err != nil {
		return nil, err
	}
	refs := make(map[string]execution.ToolRef, len(a.refs))
	for name, ref := range a.refs {
		refs[name] = ref
	}
	return &AdmittedAgent{
		dispatcher: execution.Dispatcher{Catalog: snapshot, Policy: admission.Policy, Ledger: admission.Ledger, Approver: admission.Approver, Approvals: admission.Approvals},
		refs:       refs, toolOrder: append([]string(nil), a.toolOrder...), agentType: a.agentType,
	}, nil
}

func (a *BaseAgent) CatalogSnapshot(version string) (execution.CatalogSnapshot, error) {
	return a.catalog.Snapshot(version)
}

type ToolInvocation struct {
	AttemptID      string
	RequestID      string
	Resource       string
	IdempotencyKey string
	SourceEventIDs []string
}

type invocationKey struct{}

func WithToolInvocation(ctx context.Context, invocation ToolInvocation) (context.Context, error) {
	if strings.TrimSpace(invocation.AttemptID) == "" {
		return nil, fmt.Errorf("tool invocation requires an attempt ID")
	}
	if _, exists := ctx.Value(invocationKey{}).(ToolInvocation); exists {
		return nil, fmt.Errorf("tool invocation is already pinned")
	}
	invocation.SourceEventIDs = append([]string(nil), invocation.SourceEventIDs...)
	return context.WithValue(ctx, invocationKey{}, invocation), nil
}

type AdmittedAgent struct {
	dispatcher execution.Dispatcher
	refs       map[string]execution.ToolRef
	toolOrder  []string
	agentType  string
}

func (a *AdmittedAgent) ExecuteTool(ctx context.Context, name, arguments string) (string, error) {
	ref, ok := a.refs[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q; available: %s", name, strings.Join(a.toolOrder, ", "))
	}
	invocation, ok := ctx.Value(invocationKey{}).(ToolInvocation)
	if !ok {
		return "", fmt.Errorf("tool %q has no admitted invocation metadata", name)
	}
	return a.dispatcher.ExecuteOperation(ctx, execution.OperationRequest{
		AttemptID: invocation.AttemptID, RequestID: invocation.RequestID,
		IdempotencyKey: invocation.IdempotencyKey, SourceEventIDs: invocation.SourceEventIDs,
		Tool: ref, Resource: invocation.Resource, Arguments: arguments,
	})
}

func (a *BaseAgent) HasTool(name string) bool { _, ok := a.refs[name]; return ok }
func (a *BaseAgent) ToolNames() []string      { return append([]string(nil), a.toolOrder...) }
func (a *BaseAgent) ToolCount() int           { return len(a.refs) }
func (a *BaseAgent) AgentType() string        { return a.agentType }

func (a *AdmittedAgent) HasTool(name string) bool { _, ok := a.refs[name]; return ok }
func (a *AdmittedAgent) ToolNames() []string      { return append([]string(nil), a.toolOrder...) }
func (a *AdmittedAgent) ToolCount() int           { return len(a.refs) }
func (a *AdmittedAgent) AgentType() string        { return a.agentType }
