package adapter

import (
	"context"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/trajectory"
	"github.com/anurgosw/agentic-moe/workspace"
)

type emittingHarness struct{ mockHarness }

func (*emittingHarness) Run(_ context.Context, request HarnessRequest, emit func(trajectory.Event) error) (HarnessResult, error) {
	if err := emit(trajectory.Event{ID: "e1", SessionID: request.Identity.SessionID, RunID: request.Identity.RunID, Type: "step"}); err != nil {
		return HarnessResult{}, err
	}
	return HarnessResult{Status: "complete"}, nil
}

func TestHarnessSupervisorPinsManifestAndEventIdentity(t *testing.T) {
	manifest := HarnessManifest{
		Name: "named", Version: "1", ConfigDigest: "sha256:x", Health: HealthReady,
		Capabilities: HarnessCapabilities{ResumeToken: true, ExactCheckpoint: true, IdempotentResume: true},
	}
	supervisor, err := NewHarnessSupervisor(&emittingHarness{}, manifest, 1)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := (workspace.Environment{Placement: workspace.PlacementLocal, WorkspaceLeaseID: "lease", Network: workspace.NetworkNone}).Seal()
	if err != nil {
		t.Fatal(err)
	}
	request := HarnessRequest{Identity: execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}, InputID: "i", Prompt: "do work", Model: "model", PolicyVersion: "p", CatalogVersion: "c", Environment: environment, Harness: manifest}
	count := 0
	result, err := supervisor.Run(context.Background(), request, func(trajectory.Event) error { count++; return nil })
	if err != nil || result.Status != "complete" || count != 1 {
		t.Fatalf("result=%+v count=%d err=%v", result, count, err)
	}
	request.Harness.Version = "2"
	if _, err := supervisor.Run(context.Background(), request, func(trajectory.Event) error { return nil }); err == nil {
		t.Fatal("manifest drift accepted")
	}
}

func TestHarnessSupervisorNegotiatesResumeCapability(t *testing.T) {
	manifest := HarnessManifest{
		Name: "named", Version: "1", ConfigDigest: "sha256:x", Health: HealthReady,
		Capabilities: HarnessCapabilities{ResumeToken: true, ExactCheckpoint: true, IdempotentResume: true},
	}
	supervisor, err := NewHarnessSupervisor(&emittingHarness{}, manifest, 1)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := (workspace.Environment{Placement: workspace.PlacementLocal, WorkspaceLeaseID: "lease", Network: workspace.NetworkNone}).Seal()
	if err != nil {
		t.Fatal(err)
	}
	request := HarnessRequest{
		Identity: execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}, InputID: "i", Prompt: "resume work",
		Model: "model", PolicyVersion: "p", CatalogVersion: "c", Environment: environment, Harness: manifest,
		RequiredResume: ResumeExactCheckpoint, ContinuationID: "continuation", CheckpointHash: "sha256:checkpoint",
	}
	if result, err := supervisor.Resume(context.Background(), request, func(trajectory.Event) error { return nil }); err != nil || result.Status != "complete" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	request.RequiredResume = ResumeTranscript
	if _, err := supervisor.Resume(context.Background(), request, func(trajectory.Event) error { return nil }); err == nil {
		t.Fatal("unsupported transcript resume was accepted")
	}
	request.RequiredResume = ResumeNone
	if _, err := supervisor.Resume(context.Background(), request, func(trajectory.Event) error { return nil }); err == nil {
		t.Fatal("implicit resume mode was accepted")
	}
}

func TestPausedHarnessResultPinsRecoveryEvidence(t *testing.T) {
	capabilities := HarnessCapabilities{ResumeToken: true, ExactCheckpoint: true}
	if err := validateHarnessResult(HarnessResult{
		Status: "paused", ResumeMode: ResumeExactCheckpoint, ContinuationID: "continuation", CheckpointHash: "sha256:checkpoint",
	}, capabilities); err != nil {
		t.Fatal(err)
	}
	if err := validateHarnessResult(HarnessResult{Status: "paused", ResumeMode: ResumeTranscript, ContextDigest: "sha256:context"}, capabilities); err == nil {
		t.Fatal("unsupported paused recovery mode was accepted")
	}
	if err := validateHarnessResult(HarnessResult{Status: "paused", ResumeMode: ResumeExactCheckpoint}, capabilities); err == nil {
		t.Fatal("paused result without exact checkpoint evidence was accepted")
	}
}
