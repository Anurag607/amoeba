package workspace

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

type TerminalOwner string

const (
	TerminalHost     TerminalOwner = "host"
	TerminalAgent    TerminalOwner = "agent"
	TerminalApproval TerminalOwner = "approval"
	TerminalUser     TerminalOwner = "user"
)

type TerminalLease struct {
	ID         string             `json:"id"`
	Identity   execution.Identity `json:"identity"`
	Owner      TerminalOwner      `json:"owner"`
	Generation uint64             `json:"generation"`
	Rows       int                `json:"rows,omitempty"`
	Columns    int                `json:"columns,omitempty"`
	Revision   uint64             `json:"revision"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

var ErrTerminalRevision = errors.New("terminal ownership revision conflict")

type TerminalStore interface {
	Create(context.Context, TerminalLease) (TerminalLease, error)
	CompareAndSwap(context.Context, execution.Identity, string, uint64, TerminalOwner, int, int) (TerminalLease, error)
	Get(context.Context, execution.Identity, string) (TerminalLease, bool, error)
}

type MemoryTerminalStore struct {
	mu     sync.Mutex
	leases map[string]TerminalLease
}

func NewMemoryTerminalStore() *MemoryTerminalStore {
	return &MemoryTerminalStore{leases: make(map[string]TerminalLease)}
}

func (s *MemoryTerminalStore) Create(ctx context.Context, lease TerminalLease) (TerminalLease, error) {
	if err := ctx.Err(); err != nil {
		return TerminalLease{}, err
	}
	if lease.ID == "" || lease.Identity.Validate() != nil {
		return TerminalLease{}, fmt.Errorf("terminal lease requires ID and identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.leases[lease.ID]; exists {
		return TerminalLease{}, fmt.Errorf("terminal lease %q already exists", lease.ID)
	}
	lease.Owner, lease.Generation, lease.Revision, lease.UpdatedAt = TerminalHost, 1, 1, time.Now()
	s.leases[lease.ID] = lease
	return lease, nil
}

func (s *MemoryTerminalStore) CompareAndSwap(ctx context.Context, identity execution.Identity, id string, expected uint64, owner TerminalOwner, rows, columns int) (TerminalLease, error) {
	if err := ctx.Err(); err != nil {
		return TerminalLease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	if !ok {
		return TerminalLease{}, fmt.Errorf("terminal lease %q not found", id)
	}
	if lease.Identity != identity {
		return TerminalLease{}, fmt.Errorf("terminal lease owner mismatch")
	}
	if lease.Revision != expected {
		return TerminalLease{}, ErrTerminalRevision
	}
	if !validTerminalOwner(owner) {
		return TerminalLease{}, fmt.Errorf("invalid terminal owner %q", owner)
	}
	if owner != lease.Owner {
		lease.Generation++
	}
	lease.Owner, lease.Rows, lease.Columns = owner, rows, columns
	lease.Revision++
	lease.UpdatedAt = time.Now()
	s.leases[id] = lease
	return lease, nil
}

func (s *MemoryTerminalStore) Get(ctx context.Context, identity execution.Identity, id string) (TerminalLease, bool, error) {
	if err := ctx.Err(); err != nil {
		return TerminalLease{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	if !ok {
		return TerminalLease{}, false, nil
	}
	if lease.Identity != identity {
		return TerminalLease{}, false, fmt.Errorf("terminal lease owner mismatch")
	}
	return lease, true, nil
}

type TerminalController struct{ Store TerminalStore }

func (c TerminalController) Handoff(ctx context.Context, identity execution.Identity, id string, expected uint64, to TerminalOwner) (TerminalLease, error) {
	lease, ok, err := c.load(ctx, identity, id, expected)
	if err != nil || !ok {
		return TerminalLease{}, err
	}
	if !validTerminalHandoff(lease.Owner, to) {
		return TerminalLease{}, fmt.Errorf("invalid terminal handoff %s -> %s", lease.Owner, to)
	}
	return c.Store.CompareAndSwap(ctx, identity, id, expected, to, lease.Rows, lease.Columns)
}

func (c TerminalController) Resize(ctx context.Context, identity execution.Identity, id string, expected uint64, actor TerminalOwner, rows, columns int) (TerminalLease, error) {
	lease, ok, err := c.load(ctx, identity, id, expected)
	if err != nil || !ok {
		return TerminalLease{}, err
	}
	if actor != lease.Owner {
		return TerminalLease{}, fmt.Errorf("terminal resize actor does not own input")
	}
	if rows <= 0 || columns <= 0 {
		return TerminalLease{}, fmt.Errorf("terminal dimensions must be positive")
	}
	return c.Store.CompareAndSwap(ctx, identity, id, expected, actor, rows, columns)
}

// Disconnect hands control back to the host under CAS. Stale agents cannot
// reclaim input because every ownership change increments the generation.
func (c TerminalController) Disconnect(ctx context.Context, identity execution.Identity, id string, expected uint64) (TerminalLease, error) {
	lease, ok, err := c.load(ctx, identity, id, expected)
	if err != nil || !ok {
		return TerminalLease{}, err
	}
	return c.Store.CompareAndSwap(context.WithoutCancel(ctx), identity, id, expected, TerminalHost, lease.Rows, lease.Columns)
}

func (c TerminalController) load(ctx context.Context, identity execution.Identity, id string, expected uint64) (TerminalLease, bool, error) {
	if c.Store == nil {
		return TerminalLease{}, false, fmt.Errorf("terminal store is required")
	}
	lease, ok, err := c.Store.Get(ctx, identity, id)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("terminal lease %q not found", id)
		}
		return TerminalLease{}, ok, err
	}
	if lease.Revision != expected {
		return TerminalLease{}, true, ErrTerminalRevision
	}
	return lease, true, nil
}

func validTerminalHandoff(from, to TerminalOwner) bool {
	if to == TerminalHost && from != TerminalHost {
		return true
	}
	switch from {
	case TerminalHost:
		return to == TerminalAgent || to == TerminalUser
	case TerminalAgent:
		return to == TerminalApproval || to == TerminalUser
	case TerminalApproval, TerminalUser:
		return to == TerminalAgent
	default:
		return false
	}
}

func validTerminalOwner(owner TerminalOwner) bool {
	return owner == TerminalHost || owner == TerminalAgent || owner == TerminalApproval || owner == TerminalUser
}
