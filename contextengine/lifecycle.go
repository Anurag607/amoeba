package contextengine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

// SnapshotRecord is the durable context head for one owner/session.
type SnapshotRecord struct {
	ID        string             `json:"id"`
	Identity  execution.Identity `json:"identity"`
	Snapshot  Snapshot           `json:"snapshot"`
	Digest    string             `json:"digest"`
	Revision  uint64             `json:"revision"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type SnapshotStore interface {
	Create(context.Context, SnapshotRecord) (SnapshotRecord, error)
	CompareAndSwap(context.Context, execution.Identity, string, uint64, Snapshot) (SnapshotRecord, error)
	Get(context.Context, execution.Identity, string) (SnapshotRecord, bool, error)
}

type MemorySnapshotStore struct {
	mu      sync.Mutex
	records map[string]SnapshotRecord
}

func NewMemorySnapshotStore() *MemorySnapshotStore {
	return &MemorySnapshotStore{records: make(map[string]SnapshotRecord)}
}

func (s *MemorySnapshotStore) Create(ctx context.Context, record SnapshotRecord) (SnapshotRecord, error) {
	if err := ctx.Err(); err != nil {
		return SnapshotRecord{}, err
	}
	if record.ID == "" {
		return SnapshotRecord{}, fmt.Errorf("context snapshot ID is required")
	}
	if err := record.Identity.Validate(); err != nil {
		return SnapshotRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[record.ID]; ok {
		return SnapshotRecord{}, fmt.Errorf("context snapshot %q already exists", record.ID)
	}
	record.Snapshot = cloneSnapshot(record.Snapshot)
	record.Digest, record.Revision, record.UpdatedAt = Digest(record.Snapshot), 1, time.Now()
	s.records[record.ID] = record
	return cloneSnapshotRecord(record), nil
}

func (s *MemorySnapshotStore) CompareAndSwap(ctx context.Context, owner execution.Identity, id string, expected uint64, snapshot Snapshot) (SnapshotRecord, error) {
	if err := ctx.Err(); err != nil {
		return SnapshotRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return SnapshotRecord{}, fmt.Errorf("context snapshot %q not found", id)
	}
	if record.Identity != owner {
		return SnapshotRecord{}, fmt.Errorf("context snapshot owner mismatch")
	}
	if record.Revision != expected {
		return SnapshotRecord{}, fmt.Errorf("context snapshot revision conflict")
	}
	record.Snapshot = cloneSnapshot(snapshot)
	record.Digest = Digest(snapshot)
	record.Revision++
	record.UpdatedAt = time.Now()
	s.records[id] = record
	return cloneSnapshotRecord(record), nil
}

func (s *MemorySnapshotStore) Get(ctx context.Context, owner execution.Identity, id string) (SnapshotRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return SnapshotRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return SnapshotRecord{}, false, nil
	}
	if record.Identity != owner {
		return SnapshotRecord{}, false, fmt.Errorf("context snapshot owner mismatch")
	}
	return cloneSnapshotRecord(record), true, nil
}

// Turn pins the exact context head observed before provider inference.
type Turn struct {
	Record  SnapshotRecord
	Next    Snapshot
	Updates []Update
}

type Lifecycle struct {
	Engine Engine
	Store  SnapshotStore
}

func (l Lifecycle) BeginTurn(ctx context.Context, owner execution.Identity, id, workspaceID string) (Turn, error) {
	if l.Store == nil {
		return Turn{}, fmt.Errorf("context snapshot store is required")
	}
	record, ok, err := l.Store.Get(ctx, owner, id)
	if err != nil {
		return Turn{}, err
	}
	if !ok {
		snapshot, updates, err := l.Engine.Reconcile(ctx, Snapshot{}, workspaceID)
		if err != nil {
			return Turn{}, err
		}
		record, err = l.Store.Create(ctx, SnapshotRecord{ID: id, Identity: owner, Snapshot: snapshot})
		return Turn{Record: record, Next: cloneSnapshot(snapshot), Updates: updates}, err
	}
	if Digest(record.Snapshot) != record.Digest {
		return Turn{}, fmt.Errorf("context snapshot digest mismatch")
	}
	next, updates, err := l.Engine.Reconcile(ctx, record.Snapshot, workspaceID)
	if err != nil {
		return Turn{}, err
	}
	return Turn{Record: record, Next: next, Updates: updates}, nil
}

func (l Lifecycle) Commit(ctx context.Context, turn Turn) (SnapshotRecord, error) {
	if l.Store == nil {
		return SnapshotRecord{}, fmt.Errorf("context snapshot store is required")
	}
	return l.Store.CompareAndSwap(ctx, turn.Record.Identity, turn.Record.ID, turn.Record.Revision, turn.Next)
}

func (l Lifecycle) Rebaseline(ctx context.Context, owner execution.Identity, id string, expected uint64) (SnapshotRecord, error) {
	record, ok, err := l.Store.Get(ctx, owner, id)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("context snapshot %q not found", id)
		}
		return SnapshotRecord{}, err
	}
	if record.Revision != expected {
		return SnapshotRecord{}, fmt.Errorf("context snapshot revision conflict")
	}
	return l.Store.CompareAndSwap(ctx, owner, id, expected, Rebaseline(record.Snapshot))
}

// OutputManager applies one bound and spill policy to all provider and tool
// output before it is admitted into context.
type OutputManager struct {
	MaxBytes int
	Spill    SpillStore
}

func (m OutputManager) Bound(ctx context.Context, sourceKey, revision string, class Class, trust Trust, content string) (Frame, error) {
	if sourceKey == "" || revision == "" || !validClass(class) || !validTrust(trust) {
		return Frame{}, fmt.Errorf("bounded output requires source, revision, class, and trust")
	}
	frame := Frame{SourceKey: sourceKey, Revision: revision, Class: class, Trust: trust, Content: content}
	if m.MaxBytes > 0 && len(content) > m.MaxBytes {
		if m.Spill == nil {
			return Frame{}, fmt.Errorf("output exceeds %d bytes and no spill store is configured", m.MaxBytes)
		}
		ref, err := m.Spill.Put(ctx, sourceKey, content)
		if err != nil {
			return Frame{}, fmt.Errorf("spill output %s: %w", sourceKey, err)
		}
		frame.SpillRef, frame.Content = ref, boundedPreview(content, m.MaxBytes)
	}
	return frame, nil
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := Snapshot{Epoch: in.Epoch, WorkspaceID: in.WorkspaceID, Sources: make(map[string]SourceState, len(in.Sources))}
	for key, state := range in.Sources {
		out.Sources[key] = cloneState(state)
	}
	return out
}

func cloneSnapshotRecord(in SnapshotRecord) SnapshotRecord {
	in.Snapshot = cloneSnapshot(in.Snapshot)
	return in
}
