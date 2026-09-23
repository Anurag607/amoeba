package execution

import (
	"context"
	"fmt"
)

// CanonicalAdmission is the pre-provider boundary. It pins the projections
// that must not be rediscovered or silently changed mid-run.
type CanonicalAdmission struct {
	Identity        Identity  `json:"identity"`
	InputID         string    `json:"input_id"`
	PolicyVersion   string    `json:"policy_version"`
	CatalogVersion  string    `json:"catalog_version"`
	ContextDigest   string    `json:"context_digest"`
	HarnessDigest   string    `json:"harness_digest,omitempty"`
	Reconciliation  []Attempt `json:"reconciliation,omitempty"`
	AdmissionDigest string    `json:"admission_digest"`
}

type AdmissionEnvelopeBuilder struct {
	Attempts          AttemptLedger
	MaxReconciliation int
}

func (b AdmissionEnvelopeBuilder) Build(ctx context.Context, identity Identity, inputID, policyVersion, catalogVersion, contextDigest, harnessDigest string) (CanonicalAdmission, error) {
	if err := identity.Validate(); err != nil {
		return CanonicalAdmission{}, err
	}
	if inputID == "" || policyVersion == "" || catalogVersion == "" || contextDigest == "" {
		return CanonicalAdmission{}, fmt.Errorf("canonical admission requires input, policy, catalog, and context bindings")
	}
	if b.Attempts == nil {
		return CanonicalAdmission{}, fmt.Errorf("canonical admission requires an attempt ledger")
	}
	limit := b.MaxReconciliation
	if limit <= 0 {
		limit = 32
	}
	items, err := b.Attempts.ListForReconciliation(ctx, identity, limit+1)
	if err != nil {
		return CanonicalAdmission{}, fmt.Errorf("load unresolved effects: %w", err)
	}
	if len(items) > limit {
		return CanonicalAdmission{}, fmt.Errorf("unresolved effects exceed admission projection limit %d", limit)
	}
	envelope := CanonicalAdmission{
		Identity: identity, InputID: inputID, PolicyVersion: policyVersion,
		CatalogVersion: catalogVersion, ContextDigest: contextDigest, HarnessDigest: harnessDigest,
		Reconciliation: make([]Attempt, len(items)),
	}
	for i, item := range items {
		envelope.Reconciliation[i] = cloneAttempt(item)
	}
	parts := []string{identity.PrincipalID, identity.SessionID, identity.RunID, inputID, policyVersion, catalogVersion, contextDigest, harnessDigest}
	for _, item := range items {
		parts = append(parts, item.ID, item.TargetHash, item.IdempotencyKey, string(item.State), item.ReconcileAction)
	}
	envelope.AdmissionDigest = hashSections(parts...)
	return envelope, nil
}

func (a CanonicalAdmission) ReadyForProvider() bool { return len(a.Reconciliation) == 0 }
