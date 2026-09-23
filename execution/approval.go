package execution

import (
	"context"
	"fmt"
	"time"
)

// ApprovalRequest is a reconnect-safe authorization question.
type ApprovalRequest struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Envelope     OperationEnvelope `json:"envelope"`
	EnvelopeHash string            `json:"envelope_hash"`
	ReasonCode   string            `json:"reason_code"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ApprovalDecision settles one approval request.
type ApprovalDecision struct {
	RequestID    string    `json:"request_id"`
	EnvelopeHash string    `json:"envelope_hash"`
	Allowed      bool      `json:"allowed"`
	Reason       string    `json:"reason,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// Approver may block while a durable host mediates a request. It must return
// when ctx is cancelled.
type Approver interface {
	Approve(context.Context, ApprovalRequest) (ApprovalDecision, error)
}

// NewApprovalRequest seals an approval question to the exact operation
// envelope. It is also useful to durable-store conformance suites and
// out-of-process approval presenters.
func NewApprovalRequest(id string, envelope OperationEnvelope, reason string, at time.Time) (ApprovalRequest, error) {
	if id == "" || envelope.Version != OperationEnvelopeVersion || envelope.AttemptID == "" || envelope.PolicyVersion == "" || envelope.IdempotencyKey == "" ||
		envelope.Tool.Name == "" || envelope.Tool.Version == "" || envelope.CatalogVersion == "" || envelope.SchemaDigest == "" || envelope.ArgsHash == "" || envelope.TargetHash == "" {
		return ApprovalRequest{}, fmt.Errorf("approval requires ID and a supported operation envelope")
	}
	if err := envelope.Identity.Validate(); err != nil {
		return ApprovalRequest{}, err
	}
	if err := validateSourceEvents(envelope.SourceEventIDs); err != nil {
		return ApprovalRequest{}, err
	}
	request := ApprovalRequest{Version: OperationEnvelopeVersion, ID: id, Envelope: envelope, ReasonCode: reason, CreatedAt: at}
	request.EnvelopeHash = envelopeHash(envelope)
	return request, nil
}
