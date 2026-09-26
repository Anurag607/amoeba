package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Anurag607/amoeba/guardrail"
	"github.com/Anurag607/amoeba/policy"
)

// IndeterminateError means execution may have produced an external effect;
// callers must reconcile rather than automatically retry.
type IndeterminateError struct{ Err error }

func (e *IndeterminateError) Error() string {
	if e == nil || e.Err == nil {
		return "tool outcome indeterminate"
	}
	return "tool outcome indeterminate: " + e.Err.Error()
}
func (e *IndeterminateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ExistingAttemptError struct{ Attempt Attempt }

func (e *ExistingAttemptError) Error() string {
	return fmt.Sprintf("attempt %s already exists in state %s", e.Attempt.ID, e.Attempt.State)
}

type ApprovalPendingError struct{ Request ApprovalRequest }

func (e *ApprovalPendingError) Error() string {
	return fmt.Sprintf("approval %s is pending for attempt %s", e.Request.ID, e.Request.Envelope.AttemptID)
}

// Dispatcher validates and canonicalizes before it authorizes, records, or
// exposes an approval. Every later phase consumes the same sealed envelope.
type Dispatcher struct {
	Catalog   CatalogSnapshot
	Policy    policy.Snapshot
	Ledger    AttemptLedger
	Approver  Approver
	Approvals ApprovalStore
	Now       func() time.Time
}

// Execute is the compact compatibility entry point. resource is an assertion,
// never the policy target; registered tools derive that target from arguments.
func (d Dispatcher) Execute(ctx context.Context, attemptID string, ref ToolRef, resource, arguments string) (string, error) {
	return d.ExecuteOperation(ctx, OperationRequest{
		AttemptID: attemptID, RequestID: attemptID, IdempotencyKey: attemptID,
		Tool: ref, Resource: resource, Arguments: arguments,
	})
}

// ExecuteOperation runs one request from its canonical operation envelope.
func (d Dispatcher) ExecuteOperation(ctx context.Context, request OperationRequest) (string, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("tool execution has no admitted identity")
	}
	if d.Ledger == nil {
		return "", fmt.Errorf("tool execution has no attempt ledger")
	}
	request.AttemptID = strings.TrimSpace(request.AttemptID)
	if request.AttemptID == "" {
		return "", fmt.Errorf("tool execution requires an attempt ID")
	}
	if request.RequestID == "" {
		request.RequestID = request.AttemptID
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = request.AttemptID
	}
	if err := validateSourceEvents(request.SourceEventIDs); err != nil {
		return "", err
	}
	operation, err := d.Catalog.CanonicalOperation(request.Tool, request.Resource, request.Arguments)
	if err != nil {
		return "", err
	}
	decision := decideOperation(d.Policy, operation)
	now := d.Now
	if now == nil {
		now = time.Now
	}
	envelope := OperationEnvelope{
		Version: OperationEnvelopeVersion, Identity: identity, AttemptID: request.AttemptID,
		RequestID: request.RequestID, PolicyVersion: d.Policy.Version(),
		IdempotencyKey: request.IdempotencyKey, SourceEventIDs: append([]string(nil), request.SourceEventIDs...),
		Operation: operation,
	}
	attempt := Attempt{
		ID: request.AttemptID, Identity: identity, RequestID: request.RequestID, Tool: operation.Tool,
		PolicyVersion: envelope.PolicyVersion, CatalogVersion: operation.CatalogVersion,
		SchemaDigest: operation.SchemaDigest, ArgsHash: operation.ArgsHash, TargetHash: operation.TargetHash,
		IdempotencyKey: request.IdempotencyKey, SourceEventIDs: append([]string(nil), request.SourceEventIDs...),
		AdmittedAt: now(),
	}
	current, created, err := d.Ledger.Admit(ctx, attempt)
	if err != nil {
		return "", err
	}
	approvalSatisfied := false
	if !created {
		if current.State != AttemptAdmitted {
			return "", &ExistingAttemptError{Attempt: current}
		}
		if decision.Effect == policy.EffectApprove {
			approvalSatisfied, err = d.resumeApproval(ctx, envelope, decision, current, now())
			if err != nil {
				return "", err
			}
		}
	}
	if decision.Effect == policy.EffectDeny {
		_, _ = d.Ledger.Settle(ctx, identity, current.ID, current.Revision, AttemptSettlement{
			State: AttemptDenied, ReasonCode: decision.ReasonCode, At: now(),
		})
		return "", fmt.Errorf("tool %s denied: %s", request.Tool.Name, decision.ReasonCode)
	}
	if decision.Effect == policy.EffectApprove && !approvalSatisfied {
		if err := d.requestApproval(ctx, envelope, decision, current, now()); err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		_, _ = d.Ledger.Settle(context.WithoutCancel(ctx), identity, current.ID, current.Revision, AttemptSettlement{
			State: AttemptCancelledBefore, ReasonCode: "context.cancelled", At: now(),
		})
		return "", err
	}
	current, err = d.Ledger.Start(ctx, identity, current.ID, current.Revision, now())
	if err != nil {
		return "", err
	}
	spec, _ := d.Catalog.Resolve(operation.Tool)
	out, runErr := spec.Execute(ctx, operation.CanonicalArguments)
	if runErr != nil {
		state, reconcile := AttemptFailed, ""
		var indeterminate *IndeterminateError
		if errors.As(runErr, &indeterminate) || spec.Class.SideEffecting {
			state, reconcile = AttemptIndeterminate, "inspect_effect_before_retry"
		}
		_, _ = d.Ledger.Settle(context.WithoutCancel(ctx), identity, current.ID, current.Revision, AttemptSettlement{
			State: state, ReasonCode: "tool." + string(state), ReconcileAction: reconcile, At: now(),
		})
		return "", fmt.Errorf("execute tool %s: %w", request.Tool.Name, runErr)
	}
	if _, err := d.Ledger.Settle(context.WithoutCancel(ctx), identity, current.ID, current.Revision, AttemptSettlement{
		State: AttemptSucceeded, ReasonCode: "tool.succeeded", ResultHash: hashSections(out), At: now(),
	}); err != nil {
		return "", err
	}
	if !spec.Class.TrustedOutput {
		out = guardrail.WrapToolOutput(request.AttemptID, out)
	}
	return out, nil
}

func decideOperation(snapshot policy.Snapshot, operation Operation) policy.Decision {
	decision := snapshot.Decide(operation.Tool.Name, operation.Resource)
	for _, target := range operation.Authorizations {
		candidate := snapshot.Decide(target.Action, target.Resource)
		if candidate.Effect == policy.EffectDeny {
			return candidate
		}
		if candidate.Effect == policy.EffectApprove && decision.Effect == policy.EffectAllow {
			decision = candidate
		}
	}
	return decision
}

func (d Dispatcher) requestApproval(ctx context.Context, envelope OperationEnvelope, decision policy.Decision, attempt Attempt, at time.Time) error {
	request := approvalForEnvelope(envelope, decision.ReasonCode, at)
	var record ApprovalRecord
	var err error
	if d.Approvals != nil {
		record, _, err = d.Approvals.Create(ctx, request)
		if err != nil {
			return fmt.Errorf("persist approval for tool %s: %w", envelope.Tool.Name, err)
		}
	}
	if d.Approver == nil && d.Approvals != nil {
		return &ApprovalPendingError{Request: request}
	}
	if d.Approver == nil {
		_, _ = d.Ledger.Settle(ctx, envelope.Identity, attempt.ID, attempt.Revision, AttemptSettlement{
			State: AttemptDenied, ReasonCode: "approval.unavailable", At: at,
		})
		return fmt.Errorf("tool %s requires approval", envelope.Tool.Name)
	}
	approval, approveErr := d.Approver.Approve(ctx, request)
	if approveErr != nil {
		return fmt.Errorf("approve tool %s: %w", envelope.Tool.Name, approveErr)
	}
	if approval.RequestID != request.ID || approval.EnvelopeHash != request.EnvelopeHash {
		approval = ApprovalDecision{RequestID: request.ID, EnvelopeHash: request.EnvelopeHash, Reason: "approval.invalid_response"}
	}
	if !approval.ExpiresAt.IsZero() && !at.Before(approval.ExpiresAt) {
		approval.Allowed, approval.Reason = false, "approval.expired"
	}
	if d.Approvals != nil {
		if _, err := d.Approvals.Resolve(ctx, envelope.Identity, request.ID, record.Revision, approval); err != nil {
			return fmt.Errorf("settle approval for tool %s: %w", envelope.Tool.Name, err)
		}
	}
	if !approval.Allowed {
		_, _ = d.Ledger.Settle(ctx, envelope.Identity, attempt.ID, attempt.Revision, AttemptSettlement{
			State: AttemptDenied, ReasonCode: "approval.denied", At: at,
		})
		return fmt.Errorf("tool %s approval denied", envelope.Tool.Name)
	}
	return nil
}

func (d Dispatcher) resumeApproval(ctx context.Context, envelope OperationEnvelope, decision policy.Decision, attempt Attempt, at time.Time) (bool, error) {
	if d.Approvals == nil {
		return false, &ExistingAttemptError{Attempt: attempt}
	}
	request := approvalForEnvelope(envelope, decision.ReasonCode, at)
	record, found, err := d.Approvals.Get(ctx, envelope.Identity, request.ID)
	if err != nil {
		return false, err
	}
	if !found {
		record, _, err = d.Approvals.Create(ctx, request)
		if err != nil {
			return false, fmt.Errorf("repair approval for tool %s: %w", envelope.Tool.Name, err)
		}
	}
	if !sameApprovalRequest(record.Request, request) {
		return false, fmt.Errorf("approval %s does not match canonical operation", request.ID)
	}
	if record.State == ApprovalPending {
		return false, &ApprovalPendingError{Request: record.Request}
	}
	if record.State == ApprovalDenied || (!record.Decision.ExpiresAt.IsZero() && !at.Before(record.Decision.ExpiresAt)) {
		_, _ = d.Ledger.Settle(ctx, envelope.Identity, attempt.ID, attempt.Revision, AttemptSettlement{
			State: AttemptDenied, ReasonCode: "approval.denied", At: at,
		})
		return false, fmt.Errorf("tool %s approval denied", envelope.Tool.Name)
	}
	return true, nil
}

func approvalForEnvelope(envelope OperationEnvelope, reason string, at time.Time) ApprovalRequest {
	request, _ := NewApprovalRequest(envelope.AttemptID+":approval", envelope, reason, at)
	return request
}

func validateSourceEvents(ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("source event IDs must be non-empty")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("source event ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
