package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type ApprovalState string

const (
	ApprovalPending  ApprovalState = "pending"
	ApprovalApproved ApprovalState = "approved"
	ApprovalDenied   ApprovalState = "denied"
)

type ApprovalRecord struct {
	Request  ApprovalRequest  `json:"request"`
	Decision ApprovalDecision `json:"decision,omitempty"`
	State    ApprovalState    `json:"state"`
	Revision uint64           `json:"revision"`
}

var (
	ErrApprovalRevisionConflict = errors.New("approval revision conflict")
	ErrApprovalOwnerMismatch    = errors.New("approval owner mismatch")
)

// ApprovalStore is the durable, owner-scoped reconnect boundary.
type ApprovalStore interface {
	Create(context.Context, ApprovalRequest) (ApprovalRecord, bool, error)
	Resolve(context.Context, Identity, string, uint64, ApprovalDecision) (ApprovalRecord, error)
	Get(context.Context, Identity, string) (ApprovalRecord, bool, error)
}

type MemoryApprovalStore struct {
	mu      sync.Mutex
	records map[string]ApprovalRecord
}

func NewMemoryApprovalStore() *MemoryApprovalStore {
	return &MemoryApprovalStore{records: make(map[string]ApprovalRecord)}
}

func (s *MemoryApprovalStore) Create(ctx context.Context, request ApprovalRequest) (ApprovalRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRecord{}, false, err
	}
	if err := validateApprovalRequest(request); err != nil {
		return ApprovalRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[request.ID]; ok {
		if !sameApprovalRequest(existing.Request, request) {
			return ApprovalRecord{}, false, fmt.Errorf("approval request %q reused with conflicting envelope", request.ID)
		}
		return cloneApprovalRecord(existing), false, nil
	}
	request.Envelope.SourceEventIDs = append([]string(nil), request.Envelope.SourceEventIDs...)
	record := ApprovalRecord{Request: request, State: ApprovalPending, Revision: 1}
	s.records[request.ID] = record
	return cloneApprovalRecord(record), true, nil
}

func (s *MemoryApprovalStore) Resolve(ctx context.Context, owner Identity, requestID string, expected uint64, decision ApprovalDecision) (ApprovalRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[requestID]
	if !ok {
		return ApprovalRecord{}, fmt.Errorf("approval request %q not found", requestID)
	}
	if record.Request.Envelope.Identity != owner {
		return ApprovalRecord{}, ErrApprovalOwnerMismatch
	}
	if record.Revision != expected {
		return ApprovalRecord{}, ErrApprovalRevisionConflict
	}
	if record.State != ApprovalPending {
		return cloneApprovalRecord(record), fmt.Errorf("approval request %q is already %s", requestID, record.State)
	}
	if decision.RequestID != requestID || decision.EnvelopeHash != record.Request.EnvelopeHash {
		return ApprovalRecord{}, fmt.Errorf("approval response does not match canonical request envelope")
	}
	record.Decision = decision
	record.Revision++
	if decision.Allowed {
		record.State = ApprovalApproved
	} else {
		record.State = ApprovalDenied
	}
	s.records[requestID] = record
	return cloneApprovalRecord(record), nil
}

func (s *MemoryApprovalStore) Get(ctx context.Context, owner Identity, requestID string) (ApprovalRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[requestID]
	if !ok {
		return ApprovalRecord{}, false, nil
	}
	if record.Request.Envelope.Identity != owner {
		return ApprovalRecord{}, false, ErrApprovalOwnerMismatch
	}
	return cloneApprovalRecord(record), true, nil
}

func validateApprovalRequest(request ApprovalRequest) error {
	if request.Version != OperationEnvelopeVersion || request.ID == "" || request.Envelope.AttemptID == "" {
		return fmt.Errorf("approval requires a supported version, request ID, and attempt ID")
	}
	if err := request.Envelope.Identity.Validate(); err != nil {
		return err
	}
	want := envelopeHash(request.Envelope)
	if request.EnvelopeHash == "" || request.EnvelopeHash != want {
		return fmt.Errorf("approval envelope hash mismatch")
	}
	return nil
}

func sameApprovalRequest(a, b ApprovalRequest) bool {
	return a.Version == b.Version && a.ID == b.ID && a.EnvelopeHash == b.EnvelopeHash &&
		a.ReasonCode == b.ReasonCode && sameEnvelope(a.Envelope, b.Envelope)
}

func sameEnvelope(a, b OperationEnvelope) bool {
	return a.Version == b.Version && a.Identity == b.Identity && a.AttemptID == b.AttemptID &&
		a.RequestID == b.RequestID && a.PolicyVersion == b.PolicyVersion &&
		a.IdempotencyKey == b.IdempotencyKey && sameOperation(a.Operation, b.Operation) &&
		equalStrings(a.SourceEventIDs, b.SourceEventIDs)
}

func sameOperation(a, b Operation) bool {
	if a.Tool != b.Tool || a.CatalogVersion != b.CatalogVersion || a.SchemaDigest != b.SchemaDigest ||
		a.CanonicalArguments != b.CanonicalArguments || a.ArgsHash != b.ArgsHash || a.Resource != b.Resource ||
		a.TargetHash != b.TargetHash || len(a.Authorizations) != len(b.Authorizations) {
		return false
	}
	for i := range a.Authorizations {
		if a.Authorizations[i] != b.Authorizations[i] {
			return false
		}
	}
	return true
}

func envelopeHash(envelope OperationEnvelope) string {
	parts := []string{
		fmt.Sprint(envelope.Version), envelope.Identity.PrincipalID, envelope.Identity.SessionID,
		envelope.Identity.RunID, envelope.AttemptID, envelope.RequestID, envelope.PolicyVersion,
		envelope.IdempotencyKey, envelope.Tool.Name, envelope.Tool.Version, envelope.CatalogVersion,
		envelope.SchemaDigest, envelope.CanonicalArguments, envelope.ArgsHash, envelope.Resource,
		envelope.TargetHash,
	}
	parts = append(parts, envelope.SourceEventIDs...)
	for _, target := range envelope.Authorizations {
		parts = append(parts, target.Action, target.Resource)
	}
	return hashSections(parts...)
}

func cloneApprovalRecord(in ApprovalRecord) ApprovalRecord {
	in.Request.Envelope.SourceEventIDs = append([]string(nil), in.Request.Envelope.SourceEventIDs...)
	return in
}
