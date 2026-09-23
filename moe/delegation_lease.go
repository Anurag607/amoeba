package moe

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// DelegationNode identifies one child within a root run.
type DelegationNode struct {
	ID        string
	ParentID  string
	Depth     int
	ExpertID  ExpertID
	Reasoning bool
}

// DelegationLease owns cumulative resource accounting for one root run.
// Persist or reconstruct its Snapshot when a root run can be resumed later.
type DelegationLease struct {
	mu                sync.Mutex
	runID             string
	limits            DelegationLimits
	deadline          time.Time
	nodes             int
	active            int
	consumed          int
	reserved          int
	workUnits         int
	artifactBytes     int
	children          map[string]int
	reasoningChildren map[string]int
	seen              map[string]struct{}
	artifacts         map[string]DelegationArtifact
}

// NewDelegationLease creates a shared run-level lease.
func NewDelegationLease(runID string, limits DelegationLimits) *DelegationLease {
	limits = normalizeDelegationLimits(limits)
	return &DelegationLease{
		runID: strings.TrimSpace(runID), limits: limits,
		deadline: time.Now().Add(limits.Timeout),
		children: make(map[string]int), reasoningChildren: make(map[string]int), seen: make(map[string]struct{}),
		artifacts: make(map[string]DelegationArtifact),
	}
}

type delegationLeaseKey struct{}
type delegationNodeKey struct{}
type delegationTokenGrantKey struct{}
type delegationPermitKey struct{}

// WithDelegationLease installs the root run's shared lease.
func WithDelegationLease(ctx context.Context, lease *DelegationLease) context.Context {
	return context.WithValue(ctx, delegationLeaseKey{}, lease)
}

// DelegationLeaseFromContext returns the installed run-level lease.
func DelegationLeaseFromContext(ctx context.Context) *DelegationLease {
	lease, _ := ctx.Value(delegationLeaseKey{}).(*DelegationLease)
	return lease
}

// DelegationNodeFromContext returns the active child node, if any.
func DelegationNodeFromContext(ctx context.Context) (DelegationNode, bool) {
	node, ok := ctx.Value(delegationNodeKey{}).(DelegationNode)
	return node, ok
}

// DelegationTokenGrantFromContext returns the maximum child-model tokens
// reserved for the active node. Child loops must enforce this as a hard cap.
func DelegationTokenGrantFromContext(ctx context.Context) int {
	grant, _ := ctx.Value(delegationTokenGrantKey{}).(int)
	return grant
}

// RootFinalizationReserve returns tokens held back from delegated children.
func (l *DelegationLease) RootFinalizationReserve() int { return l.limits.RootFinalizationTokens }

type delegationPermit struct {
	lease        *DelegationLease
	node         DelegationNode
	grant        int
	allowedRefs  map[string]struct{}
	parent       *delegationPermit
	suspendCount int
	finished     bool
	cancel       context.CancelFunc
}

func (l *DelegationLease) begin(ctx context.Context, expertID ExpertID, fingerprint string, requestTokens int, refs []string, reasoning bool) (permit *delegationPermit, issue *DelegationPressure) {
	if err := ctx.Err(); err != nil {
		return nil, pressure("cancelled", "context", 1, 0, false, err.Error())
	}
	parent, hasParent := DelegationNodeFromContext(ctx)
	parentID, depth := "", 1
	if hasParent {
		parentID, depth = parent.ID, parent.Depth+1
	}
	l.mu.Lock()
	parentPermit, _ := ctx.Value(delegationPermitKey{}).(*delegationPermit)
	parentSuspended := false
	if parentPermit != nil && parentPermit.lease == l && !parentPermit.finished {
		if parentPermit.suspendCount == 0 && l.active > 0 {
			l.active--
			parentSuspended = true
		}
		parentPermit.suspendCount++
	}
	defer func() {
		if issue != nil && parentPermit != nil && parentPermit.lease == l && parentPermit.suspendCount > 0 {
			parentPermit.suspendCount--
			if parentPermit.suspendCount == 0 && parentSuspended {
				l.active++
			}
		}
		l.mu.Unlock()
	}()
	if time.Now().After(l.deadline) {
		return nil, pressure("deadline", "duration", 1, 0, false, "finish the root run without more delegation")
	}
	if requestTokens > l.limits.MaxRequestTokens {
		return nil, pressure("request_oversize", "request_tokens", requestTokens, l.limits.MaxRequestTokens, true, "select fewer or smaller context fragments")
	}
	if depth > l.limits.MaxDepth {
		return nil, pressure("depth", "depth", depth, l.limits.MaxDepth, false, "return the current artifact to the parent")
	}
	if reasoning && depth > l.limits.MaxReasoningDepth {
		return nil, pressure("reasoning_depth", "reasoning_depth", depth, l.limits.MaxReasoningDepth, false, "return the current reasoning artifact to the parent")
	}
	if l.nodes >= l.limits.MaxNodes {
		return nil, pressure("nodes", "nodes", l.nodes+1, l.limits.MaxNodes, false, "synthesize existing artifacts")
	}
	if l.active >= l.limits.MaxParallel {
		return nil, pressure("parallel", "active_children", l.active+1, l.limits.MaxParallel, true, "wait for an active child before delegating again")
	}
	if l.children[parentID] >= l.limits.MaxChildren {
		return nil, pressure("children", "children", l.children[parentID]+1, l.limits.MaxChildren, false, "merge related work into existing children")
	}
	if reasoning && l.reasoningChildren[parentID] >= l.limits.MaxReasoningChildren {
		return nil, pressure("reasoning_children", "reasoning_children", l.reasoningChildren[parentID]+1, l.limits.MaxReasoningChildren, false, "synthesize existing reasoning children")
	}
	workUnits := (requestTokens + 511) / 512
	if workUnits < 1 {
		workUnits = 1
	}
	if l.workUnits+workUnits > l.limits.MaxWorkUnits {
		return nil, pressure("work_units", "work_units", l.workUnits+workUnits, l.limits.MaxWorkUnits, false, "finalize from retained artifacts")
	}
	key := parentID + "\x00" + string(expertID) + "\x00" + fingerprint
	if _, exists := l.seen[key]; exists {
		return nil, pressure("duplicate", "request", 1, 1, false, "reuse the prior artifact instead of repeating the child")
	}
	grant := l.limits.MaxNodeTokens
	delegatedLimit := l.limits.MaxTotalTokens - l.limits.RootFinalizationTokens
	remaining := delegatedLimit - l.consumed - l.reserved
	if grant > remaining {
		grant = remaining
	}
	if grant <= 0 {
		return nil, pressure("tokens", "delegated_tokens", l.consumed+l.reserved, delegatedLimit, false, "finalize from retained artifacts")
	}
	l.nodes++
	l.active++
	l.children[parentID]++
	if reasoning {
		l.reasoningChildren[parentID]++
	}
	l.workUnits += workUnits
	l.seen[key] = struct{}{}
	l.reserved += grant
	node := DelegationNode{ID: delegationNodeID(l.runID, parentID, expertID, fingerprint, l.nodes), ParentID: parentID, Depth: depth, ExpertID: expertID, Reasoning: reasoning}
	allowed := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		allowed[ref] = struct{}{}
	}
	return &delegationPermit{lease: l, node: node, grant: grant, allowedRefs: allowed, parent: parentPermit}, nil
}

func (p *delegationPermit) context(ctx context.Context) context.Context {
	ctx = WithDelegationLease(ctx, p.lease)
	ctx = context.WithValue(ctx, delegationNodeKey{}, p.node)
	ctx = context.WithValue(ctx, delegationTokenGrantKey{}, p.grant)
	ctx = context.WithValue(ctx, delegationPermitKey{}, p)
	if deadline, ok := ctx.Deadline(); !ok || p.lease.deadline.Before(deadline) {
		ctx, p.cancel = context.WithDeadline(ctx, p.lease.deadline)
	}
	return ctx
}

func (p *delegationPermit) finish(result *DelegateResult) (*DelegationArtifact, *DelegationPressure) {
	p.lease.mu.Lock()
	defer p.lease.mu.Unlock()
	if p.finished {
		return nil, pressure("finished", "permit", 1, 1, false, "do not finish a delegation twice")
	}
	p.finished = true
	if p.cancel != nil {
		p.cancel()
	}
	if p.lease.active > 0 {
		p.lease.active--
	}
	if p.parent != nil && p.parent.lease == p.lease && p.parent.suspendCount > 0 {
		p.parent.suspendCount--
		if p.parent.suspendCount == 0 && !p.parent.finished {
			p.lease.active++
		}
	}
	p.lease.reserved -= p.grant
	actual := p.grant
	if result != nil && result.Usage.TotalTokens > 0 {
		actual = result.Usage.TotalTokens
	}
	p.lease.consumed += actual
	delegatedLimit := p.lease.limits.MaxTotalTokens - p.lease.limits.RootFinalizationTokens
	if p.lease.consumed > delegatedLimit {
		return nil, pressure("tokens", "delegated_tokens", p.lease.consumed, delegatedLimit, false, "finalize from retained artifacts")
	}
	if result == nil {
		return nil, nil
	}
	artifact := result.Artifact
	artifact.Version = delegationArtifactVersion
	artifact.NodeID = p.node.ID
	artifact.ParentNodeID = p.node.ParentID
	artifact.Depth = p.node.Depth
	artifact.ExpertID = p.node.ExpertID
	if issue := validateDelegationArtifact(artifact, p.allowedRefs, p.lease.limits); issue != nil {
		issue.NodeID = p.node.ID
		return nil, issue
	}
	payload, _ := json.Marshal(artifact)
	if p.lease.artifactBytes+len(payload) > p.lease.limits.MaxTotalArtifactBytes {
		return nil, pressure("artifact_total", "artifact_bytes", p.lease.artifactBytes+len(payload), p.lease.limits.MaxTotalArtifactBytes, false, "reuse or compact retained artifacts")
	}
	p.lease.artifactBytes += len(payload)
	retained := cloneDelegationArtifact(artifact)
	p.lease.artifacts[p.node.ID] = retained
	resultCopy := cloneDelegationArtifact(retained)
	return &resultCopy, nil
}
