// Package contextengine provides stable-keyed, revisioned context assembly.
// Storage and user presentation are host responsibilities.
package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

type Class string

const (
	ClassPolicy     Class = "policy"
	ClassRequest    Class = "request"
	ClassHistory    Class = "history"
	ClassToolResult Class = "tool_result"
	ClassMemory     Class = "memory"
	ClassRuntime    Class = "runtime"
)

type Trust string

const (
	TrustAuthoritative Trust = "authoritative"
	TrustUntrusted     Trust = "untrusted"
)

// Frame is one model-visible context contribution.
type Frame struct {
	SourceKey string `json:"source_key"`
	Revision  string `json:"revision"`
	Class     Class  `json:"class"`
	Trust     Trust  `json:"trust"`
	Content   string `json:"content"`
	SpillRef  string `json:"spill_ref,omitempty"`
}

// Source is a stable-keyed context contributor.
type Source interface {
	Key() string
	Revision(context.Context) (string, error)
	Resolve(context.Context) ([]Frame, error)
}

type Registry struct {
	mu      sync.RWMutex
	sources map[string]Source
}

func NewRegistry() *Registry { return &Registry{sources: make(map[string]Source)} }
func (r *Registry) Register(source Source) error {
	if source == nil || strings.TrimSpace(source.Key()) == "" {
		return fmt.Errorf("context source key is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[source.Key()]; exists {
		return fmt.Errorf("context source %q already registered", source.Key())
	}
	r.sources[source.Key()] = source
	return nil
}

func (r *Registry) ordered() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.sources))
	for key := range r.sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Source, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.sources[key])
	}
	return out
}

type SourceState struct {
	Revision string  `json:"revision"`
	Frames   []Frame `json:"frames"`
}

// Snapshot is the admitted context baseline for one epoch.
type Snapshot struct {
	Epoch       uint64                 `json:"epoch"`
	WorkspaceID string                 `json:"workspace_id,omitempty"`
	Sources     map[string]SourceState `json:"sources"`
}

type UpdateKind string

const (
	UpdateAdded       UpdateKind = "added"
	UpdateChanged     UpdateKind = "changed"
	UpdateRemoved     UpdateKind = "removed"
	UpdateUnavailable UpdateKind = "unavailable"
)

type Update struct {
	Kind      UpdateKind `json:"kind"`
	SourceKey string     `json:"source_key"`
	Revision  string     `json:"revision,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// SpillStore stores complete oversized frame content and returns an opaque
// managed reference.
type SpillStore interface {
	Put(context.Context, string, string) (string, error)
}

type Engine struct {
	Registry      *Registry
	MaxFrameBytes int
	Spill         SpillStore
}

// Reconcile produces one deterministic chronological update batch. Prior
// admitted state is retained when an existing source is temporarily
// unavailable; a missing initial source fails explicitly.
func (e Engine) Reconcile(ctx context.Context, previous Snapshot, workspaceID string) (Snapshot, []Update, error) {
	if e.Registry == nil {
		return Snapshot{}, nil, fmt.Errorf("context registry is required")
	}
	if previous.WorkspaceID != "" && previous.WorkspaceID != workspaceID {
		previous = Snapshot{Epoch: previous.Epoch + 1, WorkspaceID: workspaceID, Sources: make(map[string]SourceState)}
	}
	if previous.Sources == nil {
		previous.Sources = make(map[string]SourceState)
	}
	next := Snapshot{Epoch: previous.Epoch, WorkspaceID: workspaceID, Sources: make(map[string]SourceState)}
	if next.Epoch == 0 {
		next.Epoch = 1
	}
	updates := make([]Update, 0)
	seen := make(map[string]struct{})
	for _, source := range e.Registry.ordered() {
		key := source.Key()
		seen[key] = struct{}{}
		prior, existed := previous.Sources[key]
		revision, err := source.Revision(ctx)
		if err != nil {
			if !existed {
				return Snapshot{}, nil, fmt.Errorf("initial context source %s unavailable: %w", key, err)
			}
			next.Sources[key] = cloneState(prior)
			updates = append(updates, Update{Kind: UpdateUnavailable, SourceKey: key, Revision: prior.Revision, Error: err.Error()})
			continue
		}
		if strings.TrimSpace(revision) == "" {
			return Snapshot{}, nil, fmt.Errorf("context source %s returned an empty revision", key)
		}
		if existed && revision == prior.Revision {
			next.Sources[key] = cloneState(prior)
			continue
		}
		frames, err := source.Resolve(ctx)
		if err != nil {
			if !existed {
				return Snapshot{}, nil, fmt.Errorf("resolve initial context source %s: %w", key, err)
			}
			next.Sources[key] = cloneState(prior)
			updates = append(updates, Update{Kind: UpdateUnavailable, SourceKey: key, Revision: prior.Revision, Error: err.Error()})
			continue
		}
		for i := range frames {
			if !validClass(frames[i].Class) || !validTrust(frames[i].Trust) {
				return Snapshot{}, nil, fmt.Errorf("context source %s returned frame with invalid class or trust", key)
			}
			frames[i].SourceKey, frames[i].Revision = key, revision
			if e.MaxFrameBytes > 0 && len(frames[i].Content) > e.MaxFrameBytes {
				if e.Spill == nil {
					return Snapshot{}, nil, fmt.Errorf("context frame %s exceeds %d bytes and no spill store is configured", key, e.MaxFrameBytes)
				}
				ref, spillErr := e.Spill.Put(ctx, key, frames[i].Content)
				if spillErr != nil {
					return Snapshot{}, nil, fmt.Errorf("spill context frame %s: %w", key, spillErr)
				}
				frames[i].SpillRef = ref
				frames[i].Content = boundedPreview(frames[i].Content, e.MaxFrameBytes)
			}
		}
		next.Sources[key] = SourceState{Revision: revision, Frames: cloneFrames(frames)}
		kind := UpdateAdded
		if existed {
			kind = UpdateChanged
		}
		updates = append(updates, Update{Kind: kind, SourceKey: key, Revision: revision})
	}
	removed := make([]string, 0)
	for key := range previous.Sources {
		if _, ok := seen[key]; !ok {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	for _, key := range removed {
		updates = append(updates, Update{Kind: UpdateRemoved, SourceKey: key})
	}
	return next, updates, nil
}

// Rebaseline begins a new epoch after compaction or another destructive
// projection change while preserving the current admitted sources.
func Rebaseline(snapshot Snapshot) Snapshot {
	out := Snapshot{Epoch: snapshot.Epoch + 1, WorkspaceID: snapshot.WorkspaceID, Sources: make(map[string]SourceState, len(snapshot.Sources))}
	for key, state := range snapshot.Sources {
		out.Sources[key] = cloneState(state)
	}
	return out
}

func Digest(snapshot Snapshot) string {
	keys := make([]string, 0, len(snapshot.Sources))
	for key := range snapshot.Sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(snapshot.WorkspaceID)
	b.WriteByte(0)
	for _, key := range keys {
		state := snapshot.Sources[key]
		b.WriteString(key)
		b.WriteByte(0)
		b.WriteString(state.Revision)
		b.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func cloneState(in SourceState) SourceState {
	return SourceState{Revision: in.Revision, Frames: cloneFrames(in.Frames)}
}
func cloneFrames(in []Frame) []Frame { return append([]Frame(nil), in...) }
func boundedPreview(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	const marker = "\n...[spill]...\n"
	if limit <= len(marker) {
		return content[:limit]
	}
	payload := limit - len(marker)
	head := payload * 2 / 3
	tail := payload - head
	for head > 0 && !utf8.ValidString(content[:head]) {
		head--
	}
	tailStart := len(content) - tail
	for tailStart < len(content) && !utf8.RuneStart(content[tailStart]) {
		tailStart++
	}
	return content[:head] + marker + content[tailStart:]
}

func validClass(class Class) bool {
	switch class {
	case ClassPolicy, ClassRequest, ClassHistory, ClassToolResult, ClassMemory, ClassRuntime:
		return true
	default:
		return false
	}
}

func validTrust(trust Trust) bool {
	return trust == TrustAuthoritative || trust == TrustUntrusted
}
