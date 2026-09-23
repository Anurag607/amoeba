package adapter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anurgosw/agentic-moe/trajectory"
)

// HarnessSupervisor enforces manifest pinning, bounded concurrency, event
// provenance, and run cleanup around one explicitly configured harness.
type HarnessSupervisor struct {
	Harness   AgentHarness
	Manifest  HarnessManifest
	MaxActive int

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func NewHarnessSupervisor(harness AgentHarness, manifest HarnessManifest, maxActive int) (*HarnessSupervisor, error) {
	if harness == nil || maxActive <= 0 || manifest.Name == "" || manifest.Version == "" || manifest.ConfigDigest == "" {
		return nil, fmt.Errorf("harness supervisor requires harness, manifest, and positive active limit")
	}
	if harness.Name() != manifest.Name {
		return nil, fmt.Errorf("harness supervisor manifest name mismatch")
	}
	return &HarnessSupervisor{Harness: harness, Manifest: manifest, MaxActive: maxActive, active: make(map[string]context.CancelFunc)}, nil
}

func (s *HarnessSupervisor) Run(ctx context.Context, request HarnessRequest, sink func(trajectory.Event) error) (HarnessResult, error) {
	return s.execute(ctx, request, false, sink)
}

func (s *HarnessSupervisor) Resume(ctx context.Context, request HarnessRequest, sink func(trajectory.Event) error) (HarnessResult, error) {
	if request.RequiredResume == ResumeNone {
		return HarnessResult{}, fmt.Errorf("resume requires an explicit recovery mode")
	}
	if !s.Manifest.Capabilities.Supports(request.RequiredResume) {
		return HarnessResult{}, fmt.Errorf("harness does not support required resume mode %q", request.RequiredResume)
	}
	switch request.RequiredResume {
	case ResumeToken:
		if request.ResumeToken == "" {
			return HarnessResult{}, fmt.Errorf("token resume requires a token")
		}
	case ResumeTranscript:
		if request.ContextDigest == "" {
			return HarnessResult{}, fmt.Errorf("transcript resume requires a context digest")
		}
	case ResumeExactCheckpoint:
		if request.ContinuationID == "" || request.CheckpointHash == "" {
			return HarnessResult{}, fmt.Errorf("exact-checkpoint resume requires continuation ID and checkpoint hash")
		}
	default:
		return HarnessResult{}, fmt.Errorf("invalid resume mode %q", request.RequiredResume)
	}
	return s.execute(ctx, request, true, sink)
}

func (s *HarnessSupervisor) Cancel(ctx context.Context, runID string) error {
	s.mu.Lock()
	cancel := s.active[runID]
	s.mu.Unlock()
	if cancel == nil {
		return fmt.Errorf("harness run %q is not active", runID)
	}
	cancel()
	if err := s.Harness.Cancel(ctx, runID); err != nil {
		return fmt.Errorf("cancel harness run %q: %w", runID, err)
	}
	return nil
}

func (s *HarnessSupervisor) Status(ctx context.Context) (HarnessStatus, error) {
	status, err := s.Harness.Status(ctx)
	if err != nil {
		return HarnessStatus{}, err
	}
	s.mu.Lock()
	status.ActiveRuns = len(s.active)
	s.mu.Unlock()
	status.UpdatedAt = time.Now()
	return status, nil
}

func (s *HarnessSupervisor) execute(ctx context.Context, request HarnessRequest, resume bool, sink func(trajectory.Event) error) (HarnessResult, error) {
	if err := request.Identity.Validate(); err != nil {
		return HarnessResult{}, err
	}
	if request.InputID == "" || request.Prompt == "" || request.Model == "" || request.PolicyVersion == "" || request.CatalogVersion == "" || sink == nil {
		return HarnessResult{}, fmt.Errorf("harness request requires input, prompt, model, policy, catalog, and event sink")
	}
	if err := request.Environment.Validate(); err != nil {
		return HarnessResult{}, fmt.Errorf("validate harness environment: %w", err)
	}
	if !request.Deadline.IsZero() && !time.Now().Before(request.Deadline) {
		return HarnessResult{}, fmt.Errorf("harness request deadline has elapsed")
	}
	if !sameHarnessManifest(request.Harness, s.Manifest) {
		return HarnessResult{}, fmt.Errorf("harness request does not match admitted manifest")
	}
	if s.Manifest.Health != HealthReady {
		return HarnessResult{}, fmt.Errorf("admitted harness is not ready")
	}
	health, err := s.Harness.Health(ctx)
	if err != nil {
		return HarnessResult{}, fmt.Errorf("check harness health: %w", err)
	}
	if health != HealthReady {
		return HarnessResult{}, fmt.Errorf("harness health is %s", health)
	}
	runID := request.Identity.RunID
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if _, exists := s.active[runID]; exists {
		s.mu.Unlock()
		cancel()
		return HarnessResult{}, fmt.Errorf("harness run %q is already active", runID)
	}
	if len(s.active) >= s.MaxActive {
		s.mu.Unlock()
		cancel()
		return HarnessResult{}, fmt.Errorf("harness active-run limit reached")
	}
	s.active[runID] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.active, runID)
		s.mu.Unlock()
	}()

	seen := make(map[string]struct{})
	var sinkMu sync.Mutex
	checkedSink := func(event trajectory.Event) error {
		sinkMu.Lock()
		defer sinkMu.Unlock()
		if event.ID == "" || event.Type == "" || event.SessionID != request.Identity.SessionID || event.RunID != request.Identity.RunID {
			return fmt.Errorf("harness emitted event outside admitted run")
		}
		if _, duplicate := seen[event.ID]; duplicate {
			return fmt.Errorf("harness emitted duplicate event %q", event.ID)
		}
		seen[event.ID] = struct{}{}
		return sink(event)
	}
	var result HarnessResult
	var runErr error
	if resume {
		result, runErr = s.Harness.Resume(runCtx, request, checkedSink)
	} else {
		result, runErr = s.Harness.Run(runCtx, request, checkedSink)
	}
	if runErr != nil {
		return HarnessResult{}, fmt.Errorf("run harness %q: %w", s.Manifest.Name, runErr)
	}
	if err := validateHarnessResult(result, s.Manifest.Capabilities); err != nil {
		return HarnessResult{}, err
	}
	return result, nil
}

func sameHarnessManifest(a, b HarnessManifest) bool {
	return a.Name == b.Name && a.Version == b.Version && a.ConfigDigest == b.ConfigDigest && a.Capabilities == b.Capabilities
}

func validateHarnessResult(result HarnessResult, capabilities HarnessCapabilities) error {
	switch result.Status {
	case "complete", "failed", "cancelled":
	case "paused":
		if result.ResumeMode == ResumeNone || !capabilities.Supports(result.ResumeMode) {
			return fmt.Errorf("paused harness result requires a supported explicit resume mode")
		}
		switch result.ResumeMode {
		case ResumeToken:
			if result.ResumeToken == "" {
				return fmt.Errorf("paused token result requires resume token")
			}
		case ResumeTranscript:
			if result.ContextDigest == "" {
				return fmt.Errorf("paused transcript result requires context digest")
			}
		case ResumeExactCheckpoint:
			if result.ContinuationID == "" || result.CheckpointHash == "" {
				return fmt.Errorf("paused exact-checkpoint result requires continuation ID and checkpoint hash")
			}
		}
	default:
		return fmt.Errorf("invalid harness result status %q", result.Status)
	}
	if result.InputTokens < 0 || result.OutputTokens < 0 {
		return fmt.Errorf("harness token counts cannot be negative")
	}
	return nil
}
