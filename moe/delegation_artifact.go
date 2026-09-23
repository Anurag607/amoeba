package moe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const delegationArtifactVersion = 1

// ArtifactFinding is one claim and the server-selected sources supporting it.
type ArtifactFinding struct {
	Claim      string   `json:"claim"`
	SourceRefs []string `json:"source_refs,omitempty"`
}

type ArtifactSource struct {
	Ref         string `json:"ref"`
	Digest      string `json:"digest"`
	Disposition string `json:"disposition"`
}

type VerifiedEffect struct {
	TransactionID  string   `json:"transaction_id"`
	IdempotencyKey string   `json:"idempotency_key"`
	Status         string   `json:"status"`
	SourceEventIDs []string `json:"source_event_ids,omitempty"`
}

// DelegationArtifact is the only successful value allowed to cross from a
// child back to its parent. Identity fields are assigned by the lease.
type DelegationArtifact struct {
	Version            int               `json:"version"`
	NodeID             string            `json:"node_id"`
	ParentNodeID       string            `json:"parent_node_id,omitempty"`
	Depth              int               `json:"depth"`
	ExpertID           ExpertID          `json:"expert_id"`
	Goal               string            `json:"goal,omitempty"`
	Constraints        []string          `json:"constraints,omitempty"`
	Summary            string            `json:"summary"`
	Findings           []ArtifactFinding `json:"findings,omitempty"`
	Sources            []ArtifactSource  `json:"sources,omitempty"`
	SourceRefs         []string          `json:"source_refs,omitempty"`
	SourceDigest       string            `json:"source_digest,omitempty"`
	WorkingSetRevision string            `json:"working_set_revision,omitempty"`
	SnapshotRevision   uint64            `json:"snapshot_revision,omitempty"`
	Trust              string            `json:"trust,omitempty"`
	OutputTokens       int               `json:"output_tokens,omitempty"`
	VerifiedEffects    []VerifiedEffect  `json:"verified_effects,omitempty"`
	Conflicts          []string          `json:"conflicts,omitempty"`
	ArtifactRefs       []string          `json:"artifact_refs,omitempty"`
	Coverage           []string          `json:"coverage"`
	OpenQuestions      []string          `json:"open_questions,omitempty"`
	NextActions        []string          `json:"next_actions,omitempty"`
}

// DelegationArtifactRef returns the canonical provenance reference a child
// uses when it relies on a retained artifact from an earlier node.
func DelegationArtifactRef(artifact DelegationArtifact) string {
	return "artifact:" + artifact.NodeID
}

// DelegationUsage reports child-model consumption to the shared lease.
type DelegationUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// DelegateResult is returned by a consumer's child-loop callback.
type DelegateResult struct {
	Artifact DelegationArtifact `json:"artifact"`
	Usage    DelegationUsage    `json:"usage"`
}

// DelegationOutcome is the JSON tool result returned to the parent.
type DelegationOutcome struct {
	Kind     string              `json:"kind"`
	Artifact *DelegationArtifact `json:"artifact,omitempty"`
	Pressure *DelegationPressure `json:"pressure,omitempty"`
}

// DelegationPressure is a typed, model-actionable resource or contract error.
type DelegationPressure struct {
	Code      string `json:"code"`
	NodeID    string `json:"node_id,omitempty"`
	Resource  string `json:"resource"`
	Used      int    `json:"used"`
	Limit     int    `json:"limit"`
	Retryable bool   `json:"retryable"`
	Guidance  string `json:"guidance"`
}

func (p *DelegationPressure) Error() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("delegation pressure %s: %s=%d limit=%d", p.Code, p.Resource, p.Used, p.Limit)
}

func (l *DelegationLease) retainedArtifact(artifact DelegationArtifact) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	stored, ok := l.artifacts[artifact.NodeID]
	if !ok {
		return false
	}
	a, _ := json.Marshal(stored)
	b, _ := json.Marshal(artifact)
	return string(a) == string(b)
}

func validateDelegationArtifact(artifact DelegationArtifact, allowed map[string]struct{}, limits DelegationLimits) *DelegationPressure {
	if strings.TrimSpace(artifact.Summary) == "" {
		return pressure("artifact_invalid", "summary", 0, 1, true, "return a non-empty summary")
	}
	if len(artifact.Coverage) == 0 {
		return pressure("artifact_invalid", "coverage", 0, 1, true, "state what the child did and did not cover")
	}
	if len(allowed) > 0 && len(artifact.SourceRefs) == 0 {
		return pressure("artifact_invalid", "source_refs", 0, 1, true, "cite the selected context or retained artifacts used")
	}
	seenRefs := make(map[string]struct{}, len(artifact.SourceRefs))
	for _, ref := range artifact.SourceRefs {
		if strings.TrimSpace(ref) == "" {
			return pressure("artifact_invalid", "source_refs", 0, 1, true, "source references must be non-empty")
		}
		if _, ok := allowed[ref]; !ok {
			return pressure("artifact_untrusted", "source_refs", 1, len(allowed), true, "cite only server-selected context or retained artifacts")
		}
		if _, duplicate := seenRefs[ref]; duplicate {
			return pressure("artifact_invalid", "source_refs", 2, 1, true, "deduplicate source references")
		}
		seenRefs[ref] = struct{}{}
	}
	for _, covered := range artifact.Coverage {
		if strings.TrimSpace(covered) == "" {
			return pressure("artifact_invalid", "coverage", 0, 1, true, "coverage entries must be non-empty")
		}
	}
	for _, finding := range artifact.Findings {
		if strings.TrimSpace(finding.Claim) == "" {
			return pressure("artifact_invalid", "finding", 0, 1, true, "give every finding a claim")
		}
		for _, ref := range finding.SourceRefs {
			if _, ok := seenRefs[ref]; !ok {
				return pressure("artifact_untrusted", "finding_source", 1, len(seenRefs), true, "cite a source declared by the artifact")
			}
		}
	}
	for _, effect := range artifact.VerifiedEffects {
		if strings.TrimSpace(effect.TransactionID) == "" || strings.TrimSpace(effect.IdempotencyKey) == "" || strings.TrimSpace(effect.Status) == "" {
			return pressure("artifact_invalid", "verified_effect", 0, 1, true, "bind every verified effect to transaction, idempotency key, and status")
		}
	}
	items := len(artifact.Findings) + len(artifact.SourceRefs) + len(artifact.Sources) + len(artifact.Coverage) + len(artifact.OpenQuestions) + len(artifact.NextActions) + len(artifact.VerifiedEffects) + len(artifact.Conflicts) + len(artifact.ArtifactRefs)
	if items > limits.MaxArtifactItems {
		return pressure("artifact_items", "artifact_items", items, limits.MaxArtifactItems, true, "merge overlapping findings")
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		return pressure("artifact_invalid", "serialization", 1, 0, true, "return a JSON-serializable artifact")
	}
	if len(payload) > limits.MaxArtifactBytes {
		return pressure("artifact_oversize", "artifact_bytes", len(payload), limits.MaxArtifactBytes, true, "return a smaller evidence-preserving artifact")
	}
	return nil
}

func pressure(code, resource string, used, limit int, retryable bool, guidance string) *DelegationPressure {
	return &DelegationPressure{Code: code, Resource: resource, Used: used, Limit: limit, Retryable: retryable, Guidance: guidance}
}

func delegationNodeID(runID, parent string, expert ExpertID, fingerprint string, ordinal int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", runID, parent, expert, fingerprint, ordinal)))
	return "delegate-" + hex.EncodeToString(sum[:8])
}

func estimateTokens(s string) int {
	n := len([]rune(s)) / 4
	if n < 1 {
		return 1
	}
	return n
}

func cloneDelegationArtifact(in DelegationArtifact) DelegationArtifact {
	out := in
	out.Findings = make([]ArtifactFinding, len(in.Findings))
	for i, finding := range in.Findings {
		out.Findings[i] = finding
		out.Findings[i].SourceRefs = append([]string(nil), finding.SourceRefs...)
	}
	out.SourceRefs = append([]string(nil), in.SourceRefs...)
	out.Sources = append([]ArtifactSource(nil), in.Sources...)
	out.Constraints = append([]string(nil), in.Constraints...)
	out.VerifiedEffects = make([]VerifiedEffect, len(in.VerifiedEffects))
	for i, effect := range in.VerifiedEffects {
		out.VerifiedEffects[i] = effect
		out.VerifiedEffects[i].SourceEventIDs = append([]string(nil), effect.SourceEventIDs...)
	}
	out.Conflicts = append([]string(nil), in.Conflicts...)
	out.ArtifactRefs = append([]string(nil), in.ArtifactRefs...)
	out.Coverage = append([]string(nil), in.Coverage...)
	out.OpenQuestions = append([]string(nil), in.OpenQuestions...)
	out.NextActions = append([]string(nil), in.NextActions...)
	return out
}
