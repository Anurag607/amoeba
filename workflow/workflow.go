// Package workflow provides an optional, deterministic DAG executor. It is
// separate from execution so hosts that only need single-run orchestration do
// not inherit workflow scheduling semantics.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

type Node struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	DependsOn   []string `json:"depends_on,omitempty"`
	InputDigest string   `json:"input_digest"`
}

type Graph struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Nodes    []Node `json:"nodes"`
}

type NodeState string

const (
	NodePending       NodeState = "pending"
	NodeRunning       NodeState = "running"
	NodeSucceeded     NodeState = "succeeded"
	NodeFailed        NodeState = "failed"
	NodeSkipped       NodeState = "skipped"
	NodeIndeterminate NodeState = "indeterminate"
)

type NodeResult struct {
	State          NodeState `json:"state"`
	ArtifactDigest string    `json:"artifact_digest,omitempty"`
	FailureCode    string    `json:"failure_code,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Run struct {
	ID       string                `json:"id"`
	Owner    execution.Identity    `json:"owner"`
	Graph    Graph                 `json:"graph"`
	Nodes    map[string]NodeResult `json:"nodes"`
	Revision uint64                `json:"revision"`
}

type Store interface {
	Create(context.Context, Run) (Run, error)
	CompareAndSwap(context.Context, execution.Identity, string, uint64, string, NodeResult) (Run, error)
	Get(context.Context, execution.Identity, string) (Run, bool, error)
}

type Handler interface {
	Execute(context.Context, execution.Identity, Node) (artifactDigest string, failureCode string, err error)
}

type HandlerFunc func(context.Context, execution.Identity, Node) (string, string, error)

func (f HandlerFunc) Execute(ctx context.Context, owner execution.Identity, node Node) (string, string, error) {
	return f(ctx, owner, node)
}

type Engine struct {
	Store    Store
	Handlers map[string]Handler
}

func (e Engine) Run(ctx context.Context, run Run) (Run, error) {
	if e.Store == nil {
		return Run{}, fmt.Errorf("workflow store is required")
	}
	order, err := run.Graph.TopologicalOrder()
	if err != nil {
		return Run{}, err
	}
	current, found, err := e.Store.Get(ctx, run.Owner, run.ID)
	if err != nil {
		return Run{}, err
	}
	if !found {
		current, err = e.Store.Create(ctx, run)
		if err != nil {
			return Run{}, err
		}
	}
	for _, node := range order {
		state := current.Nodes[node.ID]
		if state.State == NodeSucceeded || state.State == NodeFailed || state.State == NodeSkipped {
			continue
		}
		if state.State == NodeRunning || state.State == NodeIndeterminate {
			return current, fmt.Errorf("workflow node %q requires reconciliation", node.ID)
		}
		if dependencyFailed(current, node) {
			current, err = e.Store.CompareAndSwap(ctx, run.Owner, run.ID, current.Revision, node.ID, NodeResult{State: NodeSkipped, FailureCode: "dependency_failed", UpdatedAt: time.Now()})
			if err != nil {
				return Run{}, err
			}
			continue
		}
		handler := e.Handlers[node.Kind]
		if handler == nil {
			return Run{}, fmt.Errorf("workflow node %q has no handler for kind %q", node.ID, node.Kind)
		}
		current, err = e.Store.CompareAndSwap(ctx, run.Owner, run.ID, current.Revision, node.ID, NodeResult{State: NodeRunning, UpdatedAt: time.Now()})
		if err != nil {
			return Run{}, err
		}
		artifact, failure, executeErr := handler.Execute(ctx, run.Owner, node)
		result := NodeResult{State: NodeSucceeded, ArtifactDigest: artifact, UpdatedAt: time.Now()}
		if executeErr != nil {
			result.State, result.FailureCode = NodeFailed, failure
			var indeterminate *execution.IndeterminateError
			if errors.As(executeErr, &indeterminate) {
				result.State = NodeIndeterminate
			}
			if result.FailureCode == "" {
				result.FailureCode = "handler_failed"
			}
		}
		current, err = e.Store.CompareAndSwap(context.WithoutCancel(ctx), run.Owner, run.ID, current.Revision, node.ID, result)
		if err != nil {
			return Run{}, err
		}
		if executeErr != nil {
			continue
		}
	}
	return current, nil
}

func (g Graph) TopologicalOrder() ([]Node, error) {
	if strings.TrimSpace(g.ID) == "" || len(g.Nodes) == 0 {
		return nil, fmt.Errorf("workflow graph requires ID and nodes")
	}
	byID := make(map[string]Node, len(g.Nodes))
	indegree := make(map[string]int, len(g.Nodes))
	dependents := make(map[string][]string)
	for _, node := range g.Nodes {
		if node.ID == "" || node.Kind == "" || node.InputDigest == "" {
			return nil, fmt.Errorf("workflow node requires ID, kind, and input digest")
		}
		if _, duplicate := byID[node.ID]; duplicate {
			return nil, fmt.Errorf("duplicate workflow node %q", node.ID)
		}
		byID[node.ID], indegree[node.ID] = node, len(node.DependsOn)
	}
	for _, node := range g.Nodes {
		seen := make(map[string]struct{}, len(node.DependsOn))
		for _, dependency := range node.DependsOn {
			if _, ok := byID[dependency]; !ok || dependency == node.ID {
				return nil, fmt.Errorf("workflow node %q has invalid dependency %q", node.ID, dependency)
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, fmt.Errorf("workflow node %q repeats dependency %q", node.ID, dependency)
			}
			seen[dependency] = struct{}{}
			dependents[dependency] = append(dependents[dependency], node.ID)
		}
	}
	ready := make([]string, 0)
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	order := make([]Node, 0, len(g.Nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, byID[id])
		for _, dependent := range dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(g.Nodes) {
		return nil, fmt.Errorf("workflow graph contains a cycle")
	}
	return order, nil
}

type MemoryStore struct {
	mu   sync.Mutex
	runs map[string]Run
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{runs: make(map[string]Run)} }

func (s *MemoryStore) Create(ctx context.Context, run Run) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if run.ID == "" || run.Owner.Validate() != nil {
		return Run{}, fmt.Errorf("workflow run requires ID and owner")
	}
	if _, err := run.Graph.TopologicalOrder(); err != nil {
		return Run{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return Run{}, fmt.Errorf("workflow run %q already exists", run.ID)
	}
	run.Nodes = make(map[string]NodeResult, len(run.Graph.Nodes))
	for _, node := range run.Graph.Nodes {
		run.Nodes[node.ID] = NodeResult{State: NodePending, UpdatedAt: time.Now()}
	}
	run.Revision = 1
	s.runs[run.ID] = cloneRun(run)
	return cloneRun(run), nil
}

func (s *MemoryStore) CompareAndSwap(ctx context.Context, owner execution.Identity, id string, expected uint64, nodeID string, result NodeResult) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Owner != owner || run.Revision != expected {
		return Run{}, fmt.Errorf("workflow run missing, owner mismatch, or revision conflict")
	}
	prior, ok := run.Nodes[nodeID]
	if !ok || !validNodeTransition(prior.State, result.State) {
		return Run{}, fmt.Errorf("invalid workflow node transition %s -> %s", prior.State, result.State)
	}
	run.Nodes[nodeID] = result
	run.Revision++
	s.runs[id] = cloneRun(run)
	return cloneRun(run), nil
}

func (s *MemoryStore) Get(ctx context.Context, owner execution.Identity, id string) (Run, bool, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, false, nil
	}
	if run.Owner != owner {
		return Run{}, false, fmt.Errorf("workflow run owner mismatch")
	}
	return cloneRun(run), true, nil
}

func dependencyFailed(run Run, node Node) bool {
	for _, dependency := range node.DependsOn {
		state := run.Nodes[dependency].State
		if state == NodeFailed || state == NodeSkipped || state == NodeIndeterminate {
			return true
		}
	}
	return false
}

func validNodeTransition(from, to NodeState) bool {
	return (from == NodePending && (to == NodeRunning || to == NodeSkipped)) ||
		(from == NodeRunning && (to == NodeSucceeded || to == NodeFailed || to == NodeIndeterminate)) ||
		(from == NodeIndeterminate && (to == NodeSucceeded || to == NodeFailed))
}

func cloneRun(run Run) Run {
	run.Graph.Nodes = append([]Node(nil), run.Graph.Nodes...)
	for index := range run.Graph.Nodes {
		run.Graph.Nodes[index].DependsOn = append([]string(nil), run.Graph.Nodes[index].DependsOn...)
	}
	nodes := make(map[string]NodeResult, len(run.Nodes))
	for id, result := range run.Nodes {
		nodes[id] = result
	}
	run.Nodes = nodes
	return run
}
