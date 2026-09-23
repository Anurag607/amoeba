package moe

import (
	"errors"
	"fmt"
	"sync"
)

// StoredDelegationLease is one revisioned durable checkpoint.
type StoredDelegationLease struct {
	Revision uint64             `json:"revision"`
	Snapshot DelegationSnapshot `json:"snapshot"`
}

var ErrLeaseRevisionConflict = errors.New("delegation lease revision conflict")

// DelegationLeaseStore prevents concurrent continuations from spending the
// same restored budget. CompareAndSwap must be atomic in durable adapters.
type DelegationLeaseStore interface {
	Create(DelegationSnapshot) (StoredDelegationLease, error)
	Load(runID string) (StoredDelegationLease, bool, error)
	CompareAndSwap(runID string, expectedRevision uint64, next DelegationSnapshot) (StoredDelegationLease, error)
}

// MemoryDelegationLeaseStore is a concurrency-safe reference implementation.
type MemoryDelegationLeaseStore struct {
	mu     sync.Mutex
	leases map[string]StoredDelegationLease
}

func NewMemoryDelegationLeaseStore() *MemoryDelegationLeaseStore {
	return &MemoryDelegationLeaseStore{leases: make(map[string]StoredDelegationLease)}
}

func (s *MemoryDelegationLeaseStore) Create(snapshot DelegationSnapshot) (StoredDelegationLease, error) {
	if snapshot.RunID == "" {
		return StoredDelegationLease{}, fmt.Errorf("create delegation lease: run_id is required")
	}
	if _, err := RestoreDelegationLease(snapshot); err != nil {
		return StoredDelegationLease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.leases[snapshot.RunID]; exists {
		return StoredDelegationLease{}, fmt.Errorf("create delegation lease %q: already exists", snapshot.RunID)
	}
	stored := StoredDelegationLease{Revision: 1, Snapshot: cloneDelegationSnapshot(snapshot)}
	s.leases[snapshot.RunID] = stored
	return cloneStoredLease(stored), nil
}

func (s *MemoryDelegationLeaseStore) Load(runID string) (StoredDelegationLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.leases[runID]
	return cloneStoredLease(stored), ok, nil
}

func (s *MemoryDelegationLeaseStore) CompareAndSwap(runID string, expectedRevision uint64, next DelegationSnapshot) (StoredDelegationLease, error) {
	if next.RunID != runID {
		return StoredDelegationLease{}, fmt.Errorf("checkpoint delegation lease: run_id changed")
	}
	if _, err := RestoreDelegationLease(next); err != nil {
		return StoredDelegationLease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.leases[runID]
	if !ok {
		return StoredDelegationLease{}, fmt.Errorf("checkpoint delegation lease %q: not found", runID)
	}
	if current.Revision != expectedRevision {
		return StoredDelegationLease{}, ErrLeaseRevisionConflict
	}
	stored := StoredDelegationLease{Revision: current.Revision + 1, Snapshot: cloneDelegationSnapshot(next)}
	s.leases[runID] = stored
	return cloneStoredLease(stored), nil
}

func cloneStoredLease(in StoredDelegationLease) StoredDelegationLease {
	in.Snapshot = cloneDelegationSnapshot(in.Snapshot)
	return in
}

func cloneDelegationSnapshot(in DelegationSnapshot) DelegationSnapshot {
	out := in
	out.Children = make(map[string]int, len(in.Children))
	for id, count := range in.Children {
		out.Children[id] = count
	}
	out.ReasoningChildren = make(map[string]int, len(in.ReasoningChildren))
	for id, count := range in.ReasoningChildren {
		out.ReasoningChildren[id] = count
	}
	out.Seen = append([]string(nil), in.Seen...)
	out.Artifacts = make(map[string]DelegationArtifact, len(in.Artifacts))
	for id, artifact := range in.Artifacts {
		out.Artifacts[id] = cloneDelegationArtifact(artifact)
	}
	return out
}
