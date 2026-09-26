package workspace

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Anurag607/amoeba/execution"
)

type WorktreeState string

const (
	WorktreeCreating WorktreeState = "creating"
	WorktreeReady    WorktreeState = "ready"
	WorktreeCleaning WorktreeState = "cleaning"
	WorktreeRemoved  WorktreeState = "removed"
	WorktreeOrphaned WorktreeState = "orphaned"
)

type Worktree struct {
	Request   WorktreeRequest `json:"request"`
	Lease     Lease           `json:"lease"`
	State     WorktreeState   `json:"state"`
	LastError string          `json:"last_error,omitempty"`
	Revision  uint64          `json:"revision"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type WorktreeDriver interface {
	Create(context.Context, WorktreeRequest) (string, error)
	Cleanup(context.Context, string) error
}

type WorktreeStore interface {
	Create(context.Context, Worktree) (Worktree, error)
	CompareAndSwap(context.Context, execution.Identity, string, uint64, WorktreeState, Lease, string) (Worktree, error)
	Get(context.Context, execution.Identity, string) (Worktree, bool, error)
	List(context.Context, execution.Identity) ([]Worktree, error)
}

type MemoryWorktreeStore struct {
	mu    sync.Mutex
	items map[string]Worktree
}

func NewMemoryWorktreeStore() *MemoryWorktreeStore {
	return &MemoryWorktreeStore{items: make(map[string]Worktree)}
}

func (s *MemoryWorktreeStore) Create(ctx context.Context, item Worktree) (Worktree, error) {
	if err := ctx.Err(); err != nil {
		return Worktree{}, err
	}
	if item.Request.ID == "" || item.Request.Owner.Validate() != nil {
		return Worktree{}, fmt.Errorf("worktree requires ID and owner")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[item.Request.ID]; ok {
		return Worktree{}, fmt.Errorf("worktree %q already exists", item.Request.ID)
	}
	item.State, item.Revision, item.UpdatedAt = WorktreeCreating, 1, time.Now()
	s.items[item.Request.ID] = item
	return item, nil
}

func (s *MemoryWorktreeStore) CompareAndSwap(ctx context.Context, owner execution.Identity, id string, expected uint64, state WorktreeState, lease Lease, lastError string) (Worktree, error) {
	if err := ctx.Err(); err != nil {
		return Worktree{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return Worktree{}, fmt.Errorf("worktree %q not found", id)
	}
	if item.Request.Owner != owner {
		return Worktree{}, fmt.Errorf("worktree owner mismatch")
	}
	if item.Revision != expected {
		return Worktree{}, fmt.Errorf("worktree revision conflict")
	}
	item.State, item.Lease, item.LastError = state, lease, lastError
	item.Revision++
	item.UpdatedAt = time.Now()
	s.items[id] = item
	return item, nil
}

func (s *MemoryWorktreeStore) Get(ctx context.Context, owner execution.Identity, id string) (Worktree, bool, error) {
	if err := ctx.Err(); err != nil {
		return Worktree{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return Worktree{}, false, nil
	}
	if item.Request.Owner != owner {
		return Worktree{}, false, fmt.Errorf("worktree owner mismatch")
	}
	return item, true, nil
}

func (s *MemoryWorktreeStore) List(ctx context.Context, owner execution.Identity) ([]Worktree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Worktree, 0)
	for _, item := range s.items {
		if item.Request.Owner == owner {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Request.ID < out[j].Request.ID })
	return out, nil
}

type ManagedWorktrees struct {
	Driver WorktreeDriver
	Store  WorktreeStore
	Leases Manager
}

func (m ManagedWorktrees) Create(ctx context.Context, request WorktreeRequest) (Lease, error) {
	if m.Driver == nil || m.Store == nil || m.Leases == nil {
		return Lease{}, fmt.Errorf("managed worktrees require driver, store, and lease manager")
	}
	item, err := m.Store.Create(ctx, Worktree{Request: request})
	if err != nil {
		return Lease{}, err
	}
	root, err := m.Driver.Create(ctx, request)
	if err != nil {
		_, _ = m.Store.CompareAndSwap(context.WithoutCancel(ctx), request.Owner, request.ID, item.Revision, WorktreeOrphaned, Lease{}, err.Error())
		return Lease{}, fmt.Errorf("create worktree %q: %w", request.ID, err)
	}
	lease := Lease{ID: request.ID + ":lease", Workspace: request.ID, Root: root, Mode: ModeMutable, Owner: request.Owner}
	if err := m.Leases.Acquire(ctx, lease); err != nil {
		_ = m.Driver.Cleanup(context.WithoutCancel(ctx), root)
		_, _ = m.Store.CompareAndSwap(context.WithoutCancel(ctx), request.Owner, request.ID, item.Revision, WorktreeOrphaned, Lease{}, err.Error())
		return Lease{}, err
	}
	stored, found, err := m.Leases.Get(ctx, lease.ID, request.Owner)
	if err != nil || !found {
		return Lease{}, fmt.Errorf("acquired worktree lease is unavailable")
	}
	if _, err := m.Store.CompareAndSwap(ctx, request.Owner, request.ID, item.Revision, WorktreeReady, stored, ""); err != nil {
		return Lease{}, err
	}
	return stored, nil
}

func (m ManagedWorktrees) Recover(ctx context.Context, owner execution.Identity) ([]Lease, error) {
	items, err := m.Store.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := make([]Lease, 0)
	for _, item := range items {
		if item.State == WorktreeReady {
			lease, ok, err := m.Leases.Get(ctx, item.Lease.ID, owner)
			if err != nil || !ok {
				_, _ = m.Store.CompareAndSwap(context.WithoutCancel(ctx), owner, item.Request.ID, item.Revision, WorktreeOrphaned, item.Lease, "lease missing during recovery")
				continue
			}
			out = append(out, lease)
		}
	}
	return out, nil
}

func (m ManagedWorktrees) Cleanup(ctx context.Context, id string, owner execution.Identity, expected uint64) error {
	item, ok, err := m.Store.Get(ctx, owner, id)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("worktree %q not found", id)
		}
		return err
	}
	if item.Revision != expected {
		return fmt.Errorf("worktree revision conflict")
	}
	item, err = m.Store.CompareAndSwap(ctx, owner, id, expected, WorktreeCleaning, item.Lease, "")
	if err != nil {
		return err
	}
	if err := m.Driver.Cleanup(ctx, item.Lease.CanonicalRoot); err != nil {
		_, _ = m.Store.CompareAndSwap(context.WithoutCancel(ctx), owner, id, item.Revision, WorktreeOrphaned, item.Lease, err.Error())
		return err
	}
	if err := m.Leases.Release(context.WithoutCancel(ctx), item.Lease.ID, owner, item.Lease.Revision); err != nil {
		return err
	}
	_, err = m.Store.CompareAndSwap(ctx, owner, id, item.Revision, WorktreeRemoved, item.Lease, "")
	return err
}
