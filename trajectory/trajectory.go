// Package trajectory defines durable, replayable run events independently of
// transport delivery or user-interface activity.
package trajectory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Reliability string

const (
	ReliabilityDurable  Reliability = "durable"
	ReliabilityDegraded Reliability = "degraded"
)

// Event is one sanitized operational fact. Data must not contain prompts,
// credentials, or complete raw tool output.
type Event struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	RunID       string                 `json:"run_id"`
	NodeID      string                 `json:"node_id,omitempty"`
	Sequence    uint64                 `json:"sequence"`
	Type        string                 `json:"type"`
	At          time.Time              `json:"at"`
	Reliability Reliability            `json:"reliability"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// Store appends before delivery and replays strictly after a cursor.
type Store interface {
	Append(context.Context, Event) (Event, error)
	Replay(context.Context, string, string, uint64, int) ([]Event, error)
}

var ErrCursorExpired = errors.New("trajectory cursor is older than retained history")

type StreamState struct {
	EarliestSequence uint64      `json:"earliest_sequence"`
	HeadSequence     uint64      `json:"head_sequence"`
	AckedSequence    uint64      `json:"acked_sequence"`
	Reliability      Reliability `json:"reliability"`
	LastError        string      `json:"last_error,omitempty"`
}

// ReplayStore adds acknowledgement, retention, and authoritative refresh
// metadata without coupling the durable event log to a transport.
type ReplayStore interface {
	Store
	Ack(context.Context, string, string, uint64) (StreamState, error)
	State(context.Context, string, string) (StreamState, error)
	SetDegraded(context.Context, string, string, string) error
}

// Redactor sanitizes structured event data before persistence.
type Redactor func(Event) Event

type Emitter struct {
	Store  Store
	Redact Redactor
}

func (e Emitter) Emit(ctx context.Context, event Event) (Event, error) {
	if e.Store == nil {
		return Event{}, fmt.Errorf("trajectory store is required")
	}
	if e.Redact != nil {
		event = e.Redact(event)
	}
	stored, err := e.Store.Append(ctx, event)
	if err != nil {
		if replay, ok := e.Store.(ReplayStore); ok {
			_ = replay.SetDegraded(context.WithoutCancel(ctx), event.SessionID, event.RunID, err.Error())
		}
		return Event{}, err
	}
	return stored, nil
}

// MemoryStore is a concurrency-safe reference store.
type MemoryStore struct {
	mu        sync.Mutex
	events    map[string][]Event
	next      map[string]uint64
	acked     map[string]uint64
	degraded  map[string]string
	maxEvents int
}

func NewMemoryStore() *MemoryStore { return NewMemoryStoreWithRetention(0) }
func NewMemoryStoreWithRetention(maxEvents int) *MemoryStore {
	return &MemoryStore{
		events: make(map[string][]Event), next: make(map[string]uint64),
		acked: make(map[string]uint64), degraded: make(map[string]string), maxEvents: maxEvents,
	}
}
func eventKey(sessionID, runID string) string { return sessionID + "\x00" + runID }

func (s *MemoryStore) Append(_ context.Context, event Event) (Event, error) {
	if event.ID == "" || event.SessionID == "" || event.RunID == "" || event.Type == "" {
		return Event{}, fmt.Errorf("trajectory event requires id, session, run, and type")
	}
	if event.Reliability != "" && event.Reliability != ReliabilityDurable && event.Reliability != ReliabilityDegraded {
		return Event{}, fmt.Errorf("invalid trajectory reliability %q", event.Reliability)
	}
	data, err := cloneData(event.Data)
	if err != nil {
		return Event{}, fmt.Errorf("clone trajectory data: %w", err)
	}
	event.Data = data
	s.mu.Lock()
	defer s.mu.Unlock()
	key := eventKey(event.SessionID, event.RunID)
	for _, existing := range s.events[key] {
		if existing.ID != event.ID {
			continue
		}
		if existing.Type != event.Type || existing.NodeID != event.NodeID || !sameData(existing.Data, event.Data) {
			return Event{}, fmt.Errorf("trajectory event %q reused with conflicting payload", event.ID)
		}
		return cloneEvent(existing)
	}
	s.next[key]++
	event.Sequence = s.next[key]
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if event.Reliability == "" {
		event.Reliability = ReliabilityDurable
	}
	s.events[key] = append(s.events[key], event)
	if s.maxEvents > 0 && len(s.events[key]) > s.maxEvents {
		s.events[key] = append([]Event(nil), s.events[key][len(s.events[key])-s.maxEvents:]...)
	}
	return cloneEvent(event)
}

func sameData(a, b map[string]interface{}) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func (s *MemoryStore) Replay(_ context.Context, sessionID, runID string, after uint64, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("replay limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.events[eventKey(sessionID, runID)]
	if len(stored) > 0 && after+1 < stored[0].Sequence {
		return nil, ErrCursorExpired
	}
	out := make([]Event, 0, limit)
	for _, event := range stored {
		if event.Sequence <= after {
			continue
		}
		var err error
		event, err = cloneEvent(event)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *MemoryStore) Ack(ctx context.Context, sessionID, runID string, sequence uint64) (StreamState, error) {
	if err := ctx.Err(); err != nil {
		return StreamState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := eventKey(sessionID, runID)
	if sequence < s.acked[key] || sequence > s.next[key] {
		return StreamState{}, fmt.Errorf("invalid trajectory acknowledgement %d", sequence)
	}
	s.acked[key] = sequence
	return s.stateLocked(key), nil
}

func (s *MemoryStore) State(ctx context.Context, sessionID, runID string) (StreamState, error) {
	if err := ctx.Err(); err != nil {
		return StreamState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked(eventKey(sessionID, runID)), nil
}

func (s *MemoryStore) SetDegraded(ctx context.Context, sessionID, runID, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := eventKey(sessionID, runID)
	if reason == "" {
		delete(s.degraded, key)
	} else {
		s.degraded[key] = reason
	}
	return nil
}

func (s *MemoryStore) stateLocked(key string) StreamState {
	earliest := s.next[key] + 1
	if events := s.events[key]; len(events) > 0 {
		earliest = events[0].Sequence
	}
	reliability := ReliabilityDurable
	if s.degraded[key] != "" {
		reliability = ReliabilityDegraded
	}
	return StreamState{EarliestSequence: earliest, HeadSequence: s.next[key], AckedSequence: s.acked[key], Reliability: reliability, LastError: s.degraded[key]}
}

func cloneData(in map[string]interface{}) (map[string]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func cloneEvent(event Event) (Event, error) {
	data, err := cloneData(event.Data)
	if err != nil {
		return Event{}, err
	}
	event.Data = data
	return event, nil
}
