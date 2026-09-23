package execution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type InputMode string

const (
	InputQueue InputMode = "queue"
	InputSteer InputMode = "steer"
)

// InputPriority declares the safe boundary at which a newly admitted input
// may affect an active session. It is persisted with the input so reconnects
// do not silently change interrupt semantics.
type InputPriority string

const (
	InputPriorityNow   InputPriority = "now"
	InputPriorityNext  InputPriority = "next"
	InputPriorityLater InputPriority = "later"
)

type InputState string

const (
	InputAdmitted  InputState = "admitted"
	InputQueued    InputState = "queued"
	InputSteering  InputState = "steering"
	InputRunning   InputState = "running"
	InputCompleted InputState = "completed"
	InputFailed    InputState = "failed"
	InputCancelled InputState = "cancelled"
)

func (s InputState) terminal() bool {
	return s == InputCompleted || s == InputFailed || s == InputCancelled
}

// AdmittedInput stores a digest, not prompt text. Hosts retain sensitive
// request bodies in their own encrypted store.
type AdmittedInput struct {
	ID            string        `json:"id"`
	Identity      Identity      `json:"identity"`
	PayloadHash   string        `json:"payload_hash"`
	Mode          InputMode     `json:"mode"`
	Priority      InputPriority `json:"priority"`
	State         InputState    `json:"state"`
	FailureCode   string        `json:"failure_code,omitempty"`
	AdmittedAt    time.Time     `json:"admitted_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Revision      uint64        `json:"revision"`
	SteersInputID string        `json:"steers_input_id,omitempty"`
}

var (
	ErrInputConflict = errors.New("input ID reused with different request")
	ErrInputRevision = errors.New("input revision conflict")
	ErrSessionBusy   = errors.New("session has an active input")
	ErrGlobalBusy    = errors.New("global run limit reached")
)

type AdmissionStore interface {
	Create(context.Context, AdmittedInput) (AdmittedInput, bool, error)
	Transition(context.Context, Identity, string, uint64, InputState, string, string) (AdmittedInput, error)
	Get(context.Context, Identity, string) (AdmittedInput, bool, error)
	Pending(context.Context, Identity, int) ([]AdmittedInput, error)
}

type MemoryAdmissionStore struct {
	mu     sync.Mutex
	inputs map[string]AdmittedInput
}

func NewMemoryAdmissionStore() *MemoryAdmissionStore {
	return &MemoryAdmissionStore{inputs: make(map[string]AdmittedInput)}
}

func (s *MemoryAdmissionStore) Create(ctx context.Context, in AdmittedInput) (AdmittedInput, bool, error) {
	if err := ctx.Err(); err != nil {
		return AdmittedInput{}, false, err
	}
	if in.ID == "" || in.PayloadHash == "" || (in.Mode != InputQueue && in.Mode != InputSteer) {
		return AdmittedInput{}, false, fmt.Errorf("input requires ID, payload hash, and mode")
	}
	priority, err := normalizeInputPriority(in.Mode, in.Priority)
	if err != nil {
		return AdmittedInput{}, false, err
	}
	in.Priority = priority
	if err := in.Identity.Validate(); err != nil {
		return AdmittedInput{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.inputs[in.ID]; ok {
		if existing.Identity != in.Identity || existing.PayloadHash != in.PayloadHash || existing.Mode != in.Mode || existing.Priority != in.Priority {
			return AdmittedInput{}, false, ErrInputConflict
		}
		return existing, false, nil
	}
	if in.AdmittedAt.IsZero() {
		in.AdmittedAt = time.Now()
	}
	in.UpdatedAt, in.State, in.Revision = in.AdmittedAt, InputAdmitted, 1
	s.inputs[in.ID] = in
	return in, true, nil
}

func (s *MemoryAdmissionStore) Transition(ctx context.Context, owner Identity, id string, expected uint64, next InputState, failure, steers string) (AdmittedInput, error) {
	if err := ctx.Err(); err != nil {
		return AdmittedInput{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	in, ok := s.inputs[id]
	if !ok {
		return AdmittedInput{}, fmt.Errorf("input %q not found", id)
	}
	if in.Identity != owner {
		return AdmittedInput{}, fmt.Errorf("input owner mismatch")
	}
	if in.Revision != expected {
		return AdmittedInput{}, ErrInputRevision
	}
	if err := validInputTransition(in.State, next); err != nil {
		return AdmittedInput{}, err
	}
	in.State, in.FailureCode, in.SteersInputID = next, failure, steers
	in.Revision++
	in.UpdatedAt = time.Now()
	s.inputs[id] = in
	return in, nil
}

func (s *MemoryAdmissionStore) Get(ctx context.Context, owner Identity, id string) (AdmittedInput, bool, error) {
	if err := ctx.Err(); err != nil {
		return AdmittedInput{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	in, ok := s.inputs[id]
	if !ok {
		return AdmittedInput{}, false, nil
	}
	if in.Identity != owner {
		return AdmittedInput{}, false, fmt.Errorf("input owner mismatch")
	}
	return in, true, nil
}

func (s *MemoryAdmissionStore) Pending(ctx context.Context, owner Identity, limit int) ([]AdmittedInput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("pending input limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AdmittedInput, 0, limit)
	for _, in := range s.inputs {
		if in.Identity == owner && !in.State.terminal() {
			out = append(out, in)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AdmittedAt.Before(out[j].AdmittedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func validInputTransition(from, to InputState) error {
	valid := false
	switch from {
	case InputAdmitted:
		valid = to == InputQueued || to == InputSteering || to == InputRunning || to == InputCancelled
	case InputQueued, InputSteering:
		valid = to == InputRunning || to == InputCancelled
	case InputRunning:
		valid = to == InputCompleted || to == InputFailed || to == InputCancelled
	}
	if !valid {
		return fmt.Errorf("invalid input transition %s -> %s", from, to)
	}
	return nil
}

type AdmissionDecision struct {
	Input    AdmittedInput
	Join     bool
	Wake     bool
	ActiveID string
	// InterruptID is populated only for an explicit now-priority input. The
	// host remains responsible for cancelling the active provider/tool work.
	InterruptID string
}

// SessionCoordinator serializes each session and bounds global running work.
// Its wake bit coalesces repeated scheduler notifications.
type SessionCoordinator struct {
	mu        sync.Mutex
	store     AdmissionStore
	maxActive int
	active    map[string]string
	pending   []queuedInput
	wakes     map[string]bool
}

type queuedInput struct {
	id       string
	owner    Identity
	priority InputPriority
}

func NewSessionCoordinator(store AdmissionStore, maxActive int) (*SessionCoordinator, error) {
	if store == nil || maxActive <= 0 {
		return nil, fmt.Errorf("coordinator requires a store and positive global limit")
	}
	return &SessionCoordinator{store: store, maxActive: maxActive, active: make(map[string]string), wakes: make(map[string]bool)}, nil
}

func (c *SessionCoordinator) Admit(ctx context.Context, in AdmittedInput) (AdmissionDecision, error) {
	stored, created, err := c.store.Create(ctx, in)
	if err != nil {
		return AdmissionDecision{}, err
	}
	if !created {
		return AdmissionDecision{Input: stored, Join: true, ActiveID: c.activeID(stored.Identity.SessionID)}, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sessionID := stored.Identity.SessionID
	activeID := c.active[sessionID]
	next := InputRunning
	steers := ""
	if activeID != "" {
		next = InputQueued
		if stored.Priority == InputPriorityNow || stored.Priority == InputPriorityNext {
			next, steers = InputSteering, activeID
		}
	} else if len(c.active) >= c.maxActive {
		next = InputQueued
	}
	stored, err = c.store.Transition(ctx, stored.Identity, stored.ID, stored.Revision, next, "", steers)
	if err != nil {
		return AdmissionDecision{}, err
	}
	if next == InputRunning {
		c.active[sessionID] = stored.ID
	} else {
		c.pending = append(c.pending, queuedInput{id: stored.ID, owner: stored.Identity, priority: stored.Priority})
	}
	wake := !c.wakes[sessionID]
	c.wakes[sessionID] = true
	decision := AdmissionDecision{Input: stored, Wake: wake, ActiveID: c.active[sessionID]}
	if activeID != "" && stored.Priority == InputPriorityNow {
		decision.InterruptID = activeID
	}
	return decision, nil
}

func (c *SessionCoordinator) Finish(ctx context.Context, owner Identity, id string, expected uint64, state InputState, failure string) (AdmittedInput, *AdmittedInput, error) {
	if state != InputCompleted && state != InputFailed && state != InputCancelled {
		return AdmittedInput{}, nil, fmt.Errorf("finish requires a terminal state")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	current, err := c.store.Transition(ctx, owner, id, expected, state, failure, "")
	if err != nil {
		return AdmittedInput{}, nil, err
	}
	if c.active[owner.SessionID] == id {
		delete(c.active, owner.SessionID)
	}
	c.wakes[owner.SessionID] = false
	var promoted *AdmittedInput
	if len(c.active) < c.maxActive {
		promoted, err = c.promoteLocked(ctx)
		if err != nil {
			return AdmittedInput{}, nil, err
		}
	}
	return current, promoted, nil
}

func (c *SessionCoordinator) ConsumeWake(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	wake := c.wakes[sessionID]
	c.wakes[sessionID] = false
	return wake
}

func (c *SessionCoordinator) Interrupt(ctx context.Context, owner Identity, id string, expected uint64) (AdmittedInput, *AdmittedInput, error) {
	return c.Finish(ctx, owner, id, expected, InputCancelled, "input.interrupted")
}

// Recover rebuilds process-local scheduling state from durable nonterminal
// inputs. Hosts call it once after loading owner-scoped pending records.
func (c *SessionCoordinator) Recover(inputs []AdmittedInput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = make(map[string]string)
	c.pending = nil
	c.wakes = make(map[string]bool)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].AdmittedAt.Before(inputs[j].AdmittedAt) })
	for _, in := range inputs {
		if in.State.terminal() {
			continue
		}
		if in.State == InputRunning {
			if c.active[in.Identity.SessionID] != "" || len(c.active) >= c.maxActive {
				return fmt.Errorf("recovered inputs violate coordinator concurrency bounds")
			}
			c.active[in.Identity.SessionID] = in.ID
		} else {
			c.pending = append(c.pending, queuedInput{id: in.ID, owner: in.Identity, priority: in.Priority})
		}
		c.wakes[in.Identity.SessionID] = true
	}
	return nil
}

func (c *SessionCoordinator) activeID(sessionID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active[sessionID]
}

func (c *SessionCoordinator) promoteLocked(ctx context.Context) (*AdmittedInput, error) {
	for index := 0; index < len(c.pending); index++ {
		best := index
		for candidate := index + 1; candidate < len(c.pending); candidate++ {
			if priorityRank(c.pending[candidate].priority) < priorityRank(c.pending[best].priority) {
				best = candidate
			}
		}
		if best != index {
			c.pending[index], c.pending[best] = c.pending[best], c.pending[index]
		}
		queued := c.pending[index]
		if c.active[queued.owner.SessionID] != "" {
			continue
		}
		c.pending = append(c.pending[:index], c.pending[index+1:]...)
		in, ok, err := c.store.Get(ctx, queued.owner, queued.id)
		if err != nil {
			return nil, err
		}
		if !ok || in.State.terminal() {
			index--
			continue
		}
		in, err = c.store.Transition(ctx, queued.owner, queued.id, in.Revision, InputRunning, "", in.SteersInputID)
		if err != nil {
			return nil, err
		}
		c.active[queued.owner.SessionID] = queued.id
		c.wakes[queued.owner.SessionID] = true
		return &in, nil
	}
	return nil, nil
}

func normalizeInputPriority(mode InputMode, priority InputPriority) (InputPriority, error) {
	if priority == "" {
		if mode == InputSteer {
			return InputPriorityNext, nil
		}
		return InputPriorityLater, nil
	}
	if priority != InputPriorityNow && priority != InputPriorityNext && priority != InputPriorityLater {
		return "", fmt.Errorf("invalid input priority %q", priority)
	}
	return priority, nil
}

func priorityRank(priority InputPriority) int {
	switch priority {
	case InputPriorityNow:
		return 0
	case InputPriorityNext:
		return 1
	default:
		return 2
	}
}
