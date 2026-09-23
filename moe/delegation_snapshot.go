package moe

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DelegationSnapshot is the durable form of a lease. Checkpoint it whenever
// the root run can pause, then restore it before processing a continuation so
// cumulative limits cannot reset.
type DelegationSnapshot struct {
	Version           int                           `json:"version"`
	RunID             string                        `json:"run_id"`
	Limits            DelegationLimits              `json:"limits"`
	Deadline          time.Time                     `json:"deadline"`
	Nodes             int                           `json:"nodes"`
	Consumed          int                           `json:"consumed_tokens"`
	WorkUnits         int                           `json:"work_units"`
	ArtifactBytes     int                           `json:"artifact_bytes"`
	Children          map[string]int                `json:"children"`
	ReasoningChildren map[string]int                `json:"reasoning_children"`
	Seen              []string                      `json:"seen_requests"`
	Artifacts         map[string]DelegationArtifact `json:"artifacts"`
}

// Snapshot returns a deep copy suitable for durable continuation state. It
// fails while children are active because in-flight reservations cannot be
// resumed safely.
func (l *DelegationLease) Snapshot() (DelegationSnapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active != 0 || l.reserved != 0 {
		return DelegationSnapshot{}, fmt.Errorf("snapshot delegation lease: %d children still active", l.active)
	}
	return l.snapshotLocked(), nil
}

// SnapshotIfSafe reports false instead of failing while a sibling or parent
// is active, making it suitable for automatic safe-boundary checkpoints.
func (l *DelegationLease) SnapshotIfSafe() (DelegationSnapshot, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active != 0 || l.reserved != 0 {
		return DelegationSnapshot{}, false, nil
	}
	return l.snapshotLocked(), true, nil
}

func (l *DelegationLease) snapshotLocked() DelegationSnapshot {
	snapshot := DelegationSnapshot{
		Version: 1, RunID: l.runID, Limits: l.limits, Deadline: l.deadline,
		Nodes: l.nodes, Consumed: l.consumed, WorkUnits: l.workUnits, ArtifactBytes: l.artifactBytes,
		Children:          make(map[string]int, len(l.children)),
		ReasoningChildren: make(map[string]int, len(l.reasoningChildren)),
		Seen:              make([]string, 0, len(l.seen)),
		Artifacts:         make(map[string]DelegationArtifact, len(l.artifacts)),
	}
	for parent, count := range l.children {
		snapshot.Children[parent] = count
	}
	for parent, count := range l.reasoningChildren {
		snapshot.ReasoningChildren[parent] = count
	}
	for key := range l.seen {
		snapshot.Seen = append(snapshot.Seen, key)
	}
	sort.Strings(snapshot.Seen)
	for id, artifact := range l.artifacts {
		snapshot.Artifacts[id] = cloneDelegationArtifact(artifact)
	}
	return snapshot
}

// RestoreDelegationLease restores cumulative budget and provenance state for
// a continuation of the same root run.
func RestoreDelegationLease(snapshot DelegationSnapshot) (*DelegationLease, error) {
	if snapshot.Version != 1 {
		return nil, fmt.Errorf("restore delegation lease: unsupported snapshot version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.RunID) == "" {
		return nil, fmt.Errorf("restore delegation lease: run_id is required")
	}
	if snapshot.Nodes < 0 || snapshot.Consumed < 0 || snapshot.WorkUnits < 0 || snapshot.ArtifactBytes < 0 {
		return nil, fmt.Errorf("restore delegation lease: counters cannot be negative")
	}
	for parent, count := range snapshot.Children {
		if count < 0 {
			return nil, fmt.Errorf("restore delegation lease: child count for %q cannot be negative", parent)
		}
	}
	for parent, count := range snapshot.ReasoningChildren {
		if count < 0 || count > snapshot.Children[parent] {
			return nil, fmt.Errorf("restore delegation lease: invalid reasoning child count for %q", parent)
		}
	}
	limits := normalizeDelegationLimits(snapshot.Limits)
	lease := &DelegationLease{
		runID: snapshot.RunID, limits: limits, deadline: snapshot.Deadline,
		nodes: snapshot.Nodes, consumed: snapshot.Consumed, workUnits: snapshot.WorkUnits, artifactBytes: snapshot.ArtifactBytes,
		children:          make(map[string]int, len(snapshot.Children)),
		reasoningChildren: make(map[string]int, len(snapshot.ReasoningChildren)),
		seen:              make(map[string]struct{}, len(snapshot.Seen)),
		artifacts:         make(map[string]DelegationArtifact, len(snapshot.Artifacts)),
	}
	if lease.deadline.IsZero() {
		lease.deadline = time.Now().Add(limits.Timeout)
	}
	for parent, count := range snapshot.Children {
		lease.children[parent] = count
	}
	for parent, count := range snapshot.ReasoningChildren {
		lease.reasoningChildren[parent] = count
	}
	for _, key := range snapshot.Seen {
		lease.seen[key] = struct{}{}
	}
	for id, artifact := range snapshot.Artifacts {
		lease.artifacts[id] = cloneDelegationArtifact(artifact)
	}
	return lease, nil
}
