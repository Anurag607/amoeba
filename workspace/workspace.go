// Package workspace defines owner-pinned workspace and environment contracts.
// Host implementations perform Git, container, and remote-runtime operations.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

type Mode string

const (
	ModeReadOnly Mode = "read_only"
	ModeMutable  Mode = "mutable"
)

// Lease is the durable workspace authority held by one child or root run.
type Lease struct {
	ID            string             `json:"id"`
	Workspace     string             `json:"workspace"`
	Root          string             `json:"root"`
	CanonicalRoot string             `json:"canonical_root"`
	Mode          Mode               `json:"mode"`
	Owner         execution.Identity `json:"owner"`
	AcquiredAt    time.Time          `json:"acquired_at"`
	ExpiresAt     time.Time          `json:"expires_at,omitempty"`
	Revision      uint64             `json:"revision"`
}

type Manager interface {
	Acquire(context.Context, Lease) error
	Release(context.Context, string, execution.Identity, uint64) error
	Get(context.Context, string, execution.Identity) (Lease, bool, error)
	List(context.Context, execution.Identity) ([]Lease, error)
}

type leaseEntry struct {
	lease Lease
	info  os.FileInfo
}

// MemoryManager keys exclusion by canonical filesystem identity, not by the
// caller's workspace label. It also notices root replacement while leased.
type MemoryManager struct {
	mu     sync.Mutex
	leases map[string]leaseEntry
}

func NewMemoryManager() *MemoryManager { return &MemoryManager{leases: make(map[string]leaseEntry)} }

func (m *MemoryManager) Acquire(ctx context.Context, lease Lease) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease.ID == "" || lease.Workspace == "" || lease.Root == "" {
		return fmt.Errorf("workspace lease requires id, workspace, and root")
	}
	if lease.Mode != ModeReadOnly && lease.Mode != ModeMutable {
		return fmt.Errorf("invalid workspace mode %q", lease.Mode)
	}
	if err := lease.Owner.Validate(); err != nil {
		return err
	}
	root, info, err := canonicalRoot(lease.Root)
	if err != nil {
		return err
	}
	lease.CanonicalRoot = root
	if lease.AcquiredAt.IsZero() {
		lease.AcquiredAt = time.Now()
	}
	if !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(lease.AcquiredAt) {
		return fmt.Errorf("workspace lease expiry must follow acquisition")
	}
	lease.Revision = 1
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.leases[lease.ID]; exists {
		return fmt.Errorf("workspace lease %q already exists", lease.ID)
	}
	for _, active := range m.leases {
		if rootsConflict(active.lease.CanonicalRoot, root) && (active.lease.Mode == ModeMutable || lease.Mode == ModeMutable) {
			return fmt.Errorf("workspace root %q already has incompatible lease %q", root, active.lease.ID)
		}
	}
	m.leases[lease.ID] = leaseEntry{lease: lease, info: info}
	return nil
}

func (m *MemoryManager) Release(ctx context.Context, id string, owner execution.Identity, expected uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.leases[id]
	if !ok {
		return fmt.Errorf("workspace lease %q not found", id)
	}
	if entry.lease.Owner != owner {
		return fmt.Errorf("workspace lease %q owner mismatch", id)
	}
	if entry.lease.Revision != expected {
		return fmt.Errorf("workspace lease %q revision conflict", id)
	}
	delete(m.leases, id)
	return nil
}

func (m *MemoryManager) Get(ctx context.Context, id string, owner execution.Identity) (Lease, bool, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.leases[id]
	if !ok {
		return Lease{}, false, nil
	}
	if entry.lease.Owner != owner {
		return Lease{}, false, fmt.Errorf("workspace lease %q owner mismatch", id)
	}
	if err := validateRootIdentity(entry); err != nil {
		return Lease{}, false, err
	}
	if !entry.lease.ExpiresAt.IsZero() && !time.Now().Before(entry.lease.ExpiresAt) {
		return Lease{}, false, fmt.Errorf("workspace lease %q expired", id)
	}
	return entry.lease, true, nil
}

func (m *MemoryManager) List(ctx context.Context, owner execution.Identity) ([]Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Lease, 0)
	for _, entry := range m.leases {
		if entry.lease.Owner == owner {
			out = append(out, entry.lease)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func canonicalRoot(root string) (string, os.FileInfo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("canonicalize workspace root: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	real = filepath.Clean(real)
	info, err := os.Stat(real)
	if err != nil {
		return "", nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("workspace root is not a directory")
	}
	return real, info, nil
}

func rootsConflict(a, b string) bool {
	if a == b {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}

func validateRootIdentity(entry leaseEntry) error {
	info, err := os.Stat(entry.lease.CanonicalRoot)
	if err != nil {
		return fmt.Errorf("workspace root is unavailable: %w", err)
	}
	if !os.SameFile(entry.info, info) {
		return fmt.Errorf("workspace root identity changed while lease was active")
	}
	return nil
}

type Placement string

const (
	PlacementLocal     Placement = "local"
	PlacementContainer Placement = "container"
	PlacementRemote    Placement = "remote"
)

type NetworkPolicy string

const (
	NetworkNone       NetworkPolicy = "none"
	NetworkRestricted NetworkPolicy = "restricted"
	NetworkHost       NetworkPolicy = "host"
)

type ResourceLimits struct {
	CPUMillis    int   `json:"cpu_millis,omitempty"`
	MemoryBytes  int64 `json:"memory_bytes,omitempty"`
	ProcessCount int   `json:"process_count,omitempty"`
}

type Repository struct {
	URL      string `json:"url"`
	Revision string `json:"revision"`
	Path     string `json:"path"`
}

// Environment is a reproducible placement description, not a sandbox claim.
type Environment struct {
	Version          int               `json:"version"`
	Placement        Placement         `json:"placement"`
	WorkspaceLeaseID string            `json:"workspace_lease_id"`
	ImageDigest      string            `json:"image_digest,omitempty"`
	Repositories     []Repository      `json:"repositories,omitempty"`
	Setup            []string          `json:"setup,omitempty"`
	Runtime          map[string]string `json:"runtime,omitempty"`
	Network          NetworkPolicy     `json:"network"`
	Resources        ResourceLimits    `json:"resources"`
	SecretBindings   []string          `json:"secret_bindings,omitempty"`
	CleanupPolicy    string            `json:"cleanup_policy,omitempty"`
	Digest           string            `json:"digest"`
}

func (e Environment) Validate() error {
	if e.Version != 1 {
		return fmt.Errorf("unsupported environment version %d", e.Version)
	}
	if e.Placement != PlacementLocal && e.Placement != PlacementContainer && e.Placement != PlacementRemote {
		return fmt.Errorf("invalid placement %q", e.Placement)
	}
	if strings.TrimSpace(e.WorkspaceLeaseID) == "" || strings.TrimSpace(e.Digest) == "" {
		return fmt.Errorf("workspace lease ID and environment digest are required")
	}
	if e.Placement == PlacementContainer && strings.TrimSpace(e.ImageDigest) == "" {
		return fmt.Errorf("container environment requires an image digest")
	}
	if e.Network != NetworkNone && e.Network != NetworkRestricted && e.Network != NetworkHost {
		return fmt.Errorf("invalid network policy %q", e.Network)
	}
	if e.Resources.CPUMillis < 0 || e.Resources.MemoryBytes < 0 || e.Resources.ProcessCount < 0 {
		return fmt.Errorf("resource limits cannot be negative")
	}
	for _, repo := range e.Repositories {
		if strings.TrimSpace(repo.URL) == "" || strings.TrimSpace(repo.Revision) == "" || strings.TrimSpace(repo.Path) == "" {
			return fmt.Errorf("repositories require URL, pinned revision, and path")
		}
	}
	want, err := e.computeDigest()
	if err != nil {
		return err
	}
	if e.Digest != want {
		return fmt.Errorf("environment digest mismatch")
	}
	return nil
}

func (e Environment) Seal() (Environment, error) {
	if e.Version == 0 {
		e.Version = 1
	}
	e.Digest = "pending"
	digest, err := e.computeDigest()
	if err != nil {
		return Environment{}, err
	}
	e.Digest = digest
	if err := e.Validate(); err != nil {
		return Environment{}, err
	}
	return e, nil
}

func (e Environment) computeDigest() (string, error) {
	e.Digest = ""
	payload, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("encode environment: %w", err)
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type WorktreeRequest struct {
	ID       string
	Owner    execution.Identity
	Source   string
	Revision string
	Path     string
}

type WorktreeManager interface {
	Create(context.Context, WorktreeRequest) (Lease, error)
	Recover(context.Context, execution.Identity) ([]Lease, error)
	Cleanup(context.Context, string, execution.Identity, uint64) error
}
