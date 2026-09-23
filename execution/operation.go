package execution

import (
	"crypto/sha256"
	"encoding/hex"
)

const OperationEnvelopeVersion = 1

// Operation is the canonical, schema-pinned value authorized by policy and
// approvals. It contains no caller-authored description of the target.
type Operation struct {
	Tool               ToolRef               `json:"tool"`
	CatalogVersion     string                `json:"catalog_version"`
	SchemaDigest       string                `json:"schema_digest"`
	CanonicalArguments string                `json:"canonical_arguments"`
	ArgsHash           string                `json:"args_hash"`
	Resource           string                `json:"resource,omitempty"`
	TargetHash         string                `json:"target_hash"`
	Authorizations     []AuthorizationTarget `json:"authorizations,omitempty"`
}

type AuthorizationTarget struct {
	Action   string `json:"action"`
	Resource string `json:"resource,omitempty"`
}

// OperationRequest binds an operation to its admitted execution provenance.
type OperationRequest struct {
	AttemptID      string   `json:"attempt_id"`
	RequestID      string   `json:"request_id,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	SourceEventIDs []string `json:"source_event_ids,omitempty"`
	Tool           ToolRef  `json:"tool"`
	Resource       string   `json:"resource,omitempty"`
	Arguments      string   `json:"arguments"`
}

// OperationEnvelope is the closed record used for intent, approval, dispatch,
// reconciliation, and audit.
type OperationEnvelope struct {
	Version        int      `json:"version"`
	Identity       Identity `json:"identity"`
	AttemptID      string   `json:"attempt_id"`
	RequestID      string   `json:"request_id,omitempty"`
	PolicyVersion  string   `json:"policy_version"`
	IdempotencyKey string   `json:"idempotency_key"`
	SourceEventIDs []string `json:"source_event_ids,omitempty"`
	Operation
}

func newOperation(ref ToolRef, catalogVersion, schemaDigest, arguments, resource string, authorizations []AuthorizationTarget) Operation {
	argsHash := hashSections(arguments)
	targetParts := []string{ref.Name, ref.Version, resource, argsHash}
	for _, target := range authorizations {
		targetParts = append(targetParts, target.Action, target.Resource)
	}
	return Operation{
		Tool: ref, CatalogVersion: catalogVersion, SchemaDigest: schemaDigest,
		CanonicalArguments: arguments, ArgsHash: argsHash, Resource: resource,
		TargetHash: hashSections(targetParts...), Authorizations: append([]AuthorizationTarget(nil), authorizations...),
	}
}

func hashSections(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		var size [8]byte
		n := uint64(len(part))
		for i := 7; i >= 0; i-- {
			size[i] = byte(n)
			n >>= 8
		}
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(part))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
