package execution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type ToolCallState string

const (
	ToolCallPlanned         ToolCallState = "planned"
	ToolCallAdmitted        ToolCallState = "admitted"
	ToolCallApprovalPending ToolCallState = "approval_pending"
	ToolCallExecuting       ToolCallState = "executing"
	ToolCallSucceeded       ToolCallState = "succeeded"
	ToolCallFailed          ToolCallState = "failed"
	ToolCallDenied          ToolCallState = "denied"
	ToolCallIndeterminate   ToolCallState = "indeterminate"
	ToolCallOmitted         ToolCallState = "omitted"
)

func (s ToolCallState) terminal() bool {
	switch s {
	case ToolCallSucceeded, ToolCallFailed, ToolCallDenied, ToolCallIndeterminate, ToolCallOmitted:
		return true
	default:
		return false
	}
}

type ToolOmission struct {
	ReasonCode string `json:"reason_code"`
	Safe       bool   `json:"safe"`
}

type ToolCallRecord struct {
	ID        string        `json:"id"`
	Tool      ToolRef       `json:"tool"`
	Required  bool          `json:"required"`
	State     ToolCallState `json:"state"`
	AttemptID string        `json:"attempt_id,omitempty"`
	Omission  *ToolOmission `json:"omission,omitempty"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ToolRound is the durable closure boundary for one model tool-call batch.
// Calls retain proposal order while settlement uses a round-wide CAS revision.
type ToolRound struct {
	ID             string           `json:"id"`
	Identity       Identity         `json:"identity"`
	PolicyVersion  string           `json:"policy_version"`
	CatalogVersion string           `json:"catalog_version"`
	Calls          []ToolCallRecord `json:"calls"`
	Revision       uint64           `json:"revision"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type ToolRoundAssessment struct {
	Converged bool     `json:"converged"`
	Blockers  []string `json:"blockers,omitempty"`
}

func (r ToolRound) Assess() ToolRoundAssessment {
	assessment := ToolRoundAssessment{Converged: true}
	for _, call := range r.Calls {
		if !call.State.terminal() {
			assessment.Converged = false
			assessment.Blockers = append(assessment.Blockers, call.ID+":nonterminal")
			continue
		}
		if call.State == ToolCallIndeterminate {
			assessment.Converged = false
			assessment.Blockers = append(assessment.Blockers, call.ID+":indeterminate")
			continue
		}
		if call.Required && call.State != ToolCallSucceeded && (call.State != ToolCallOmitted || call.Omission == nil || !call.Omission.Safe) {
			assessment.Converged = false
			assessment.Blockers = append(assessment.Blockers, call.ID+":required_unsatisfied")
		}
	}
	return assessment
}

var (
	ErrToolRoundConflict = errors.New("tool round ID reused with different intent")
	ErrToolRoundRevision = errors.New("tool round revision conflict")
)

type ToolRoundStore interface {
	Create(context.Context, ToolRound) (ToolRound, bool, error)
	Transition(context.Context, Identity, string, uint64, string, ToolCallState, string, *ToolOmission) (ToolRound, error)
	Get(context.Context, Identity, string) (ToolRound, bool, error)
	Open(context.Context, Identity, int) ([]ToolRound, error)
}

type MemoryToolRoundStore struct {
	mu     sync.Mutex
	rounds map[string]ToolRound
}

func NewMemoryToolRoundStore() *MemoryToolRoundStore {
	return &MemoryToolRoundStore{rounds: make(map[string]ToolRound)}
}

func (s *MemoryToolRoundStore) Create(ctx context.Context, round ToolRound) (ToolRound, bool, error) {
	if err := ctx.Err(); err != nil {
		return ToolRound{}, false, err
	}
	if err := validateToolRound(round); err != nil {
		return ToolRound{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.rounds[round.ID]; ok {
		if !sameToolRoundIntent(existing, round) {
			return ToolRound{}, false, ErrToolRoundConflict
		}
		return cloneToolRound(existing), false, nil
	}
	now := time.Now()
	round.Revision, round.CreatedAt, round.UpdatedAt = 1, now, now
	for index := range round.Calls {
		round.Calls[index].State = ToolCallPlanned
		round.Calls[index].UpdatedAt = now
		round.Calls[index].AttemptID = ""
		round.Calls[index].Omission = nil
	}
	s.rounds[round.ID] = cloneToolRound(round)
	return cloneToolRound(round), true, nil
}

func (s *MemoryToolRoundStore) Transition(ctx context.Context, owner Identity, roundID string, expected uint64, callID string, next ToolCallState, attemptID string, omission *ToolOmission) (ToolRound, error) {
	if err := ctx.Err(); err != nil {
		return ToolRound{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	round, ok := s.rounds[roundID]
	if !ok {
		return ToolRound{}, fmt.Errorf("tool round %q not found", roundID)
	}
	if round.Identity != owner {
		return ToolRound{}, fmt.Errorf("tool round owner mismatch")
	}
	if round.Revision != expected {
		return ToolRound{}, ErrToolRoundRevision
	}
	index := -1
	for i := range round.Calls {
		if round.Calls[i].ID == callID {
			index = i
			break
		}
	}
	if index < 0 {
		return ToolRound{}, fmt.Errorf("tool call %q not found", callID)
	}
	call := round.Calls[index]
	if err := validateToolCallTransition(call.State, next); err != nil {
		return ToolRound{}, err
	}
	if next == ToolCallOmitted {
		if omission == nil || omission.ReasonCode == "" {
			return ToolRound{}, fmt.Errorf("omitted tool call requires an explicit reason")
		}
		copy := *omission
		call.Omission = &copy
	} else if omission != nil {
		return ToolRound{}, fmt.Errorf("tool omission is only valid for omitted calls")
	}
	if next == ToolCallAdmitted || next == ToolCallApprovalPending || next == ToolCallExecuting || next == ToolCallSucceeded || next == ToolCallFailed || next == ToolCallIndeterminate {
		if attemptID == "" {
			return ToolRound{}, fmt.Errorf("tool state %s requires an attempt ID", next)
		}
		if call.AttemptID != "" && call.AttemptID != attemptID {
			return ToolRound{}, fmt.Errorf("tool call attempt cannot be rebound")
		}
		call.AttemptID = attemptID
	}
	call.State, call.UpdatedAt = next, time.Now()
	round.Calls[index] = call
	round.Revision++
	round.UpdatedAt = call.UpdatedAt
	s.rounds[roundID] = cloneToolRound(round)
	return cloneToolRound(round), nil
}

func (s *MemoryToolRoundStore) Get(ctx context.Context, owner Identity, id string) (ToolRound, bool, error) {
	if err := ctx.Err(); err != nil {
		return ToolRound{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	round, ok := s.rounds[id]
	if !ok {
		return ToolRound{}, false, nil
	}
	if round.Identity != owner {
		return ToolRound{}, false, fmt.Errorf("tool round owner mismatch")
	}
	return cloneToolRound(round), true, nil
}

func (s *MemoryToolRoundStore) Open(ctx context.Context, owner Identity, limit int) ([]ToolRound, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("tool round limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ToolRound, 0, limit)
	for _, round := range s.rounds {
		if round.Identity == owner && !round.Assess().Converged {
			out = append(out, cloneToolRound(round))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func validateToolRound(round ToolRound) error {
	if round.ID == "" || round.PolicyVersion == "" || round.CatalogVersion == "" || len(round.Calls) == 0 {
		return fmt.Errorf("tool round requires ID, policy, catalog, and calls")
	}
	if err := round.Identity.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(round.Calls))
	for _, call := range round.Calls {
		if call.ID == "" || call.Tool.Name == "" || call.Tool.Version == "" {
			return fmt.Errorf("tool round call requires ID and pinned tool")
		}
		if _, duplicate := seen[call.ID]; duplicate {
			return fmt.Errorf("duplicate tool call %q", call.ID)
		}
		seen[call.ID] = struct{}{}
	}
	return nil
}

func validateToolCallTransition(from, to ToolCallState) error {
	valid := false
	switch from {
	case ToolCallPlanned:
		valid = to == ToolCallAdmitted || to == ToolCallOmitted || to == ToolCallDenied
	case ToolCallAdmitted:
		valid = to == ToolCallApprovalPending || to == ToolCallExecuting || to == ToolCallDenied || to == ToolCallOmitted
	case ToolCallApprovalPending:
		valid = to == ToolCallExecuting || to == ToolCallDenied
	case ToolCallExecuting:
		valid = to == ToolCallSucceeded || to == ToolCallFailed || to == ToolCallIndeterminate
	}
	if !valid {
		return fmt.Errorf("invalid tool call transition %s -> %s", from, to)
	}
	return nil
}

func sameToolRoundIntent(a, b ToolRound) bool {
	if a.ID != b.ID || a.Identity != b.Identity || a.PolicyVersion != b.PolicyVersion || a.CatalogVersion != b.CatalogVersion || len(a.Calls) != len(b.Calls) {
		return false
	}
	for i := range a.Calls {
		if a.Calls[i].ID != b.Calls[i].ID || a.Calls[i].Tool != b.Calls[i].Tool || a.Calls[i].Required != b.Calls[i].Required {
			return false
		}
	}
	return true
}

func cloneToolRound(round ToolRound) ToolRound {
	round.Calls = append([]ToolCallRecord(nil), round.Calls...)
	for index := range round.Calls {
		if round.Calls[index].Omission != nil {
			copy := *round.Calls[index].Omission
			round.Calls[index].Omission = &copy
		}
	}
	return round
}
