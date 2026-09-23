package conformance

import (
	"context"
	"errors"
	"testing"

	"github.com/anurgosw/agentic-moe/contextengine"
	"github.com/anurgosw/agentic-moe/continuation"
	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/trajectory"
)

func TestMemoryStoresMeetCoreConformance(t *testing.T) {
	err := ProbeCore(context.Background(), memoryCoreStores())
	if err != nil {
		t.Fatal(err)
	}
}

func TestProbeRestartReportsReopenedCloseFailure(t *testing.T) {
	stores := memoryCoreStores()
	closeFailure := errors.New("close failed")
	var opens int
	err := ProbeRestart(context.Background(), RestartHarness{Open: func(context.Context) (CoreStores, func() error, error) {
		opens++
		if opens == 2 {
			return stores, func() error { return closeFailure }, nil
		}
		return stores, func() error { return nil }, nil
	}})
	if !errors.Is(err, closeFailure) {
		t.Fatalf("ProbeRestart() error = %v", err)
	}
}

func memoryCoreStores() CoreStores {
	return CoreStores{
		Admissions: execution.NewMemoryAdmissionStore(), Runs: execution.NewMemoryRunStore(), Attempts: execution.NewMemoryAttemptLedger(),
		Approvals: execution.NewMemoryApprovalStore(), ToolRounds: execution.NewMemoryToolRoundStore(), Work: execution.NewMemoryWorkStore(),
		Contexts: contextengine.NewMemorySnapshotStore(), Trajectory: trajectory.NewMemoryStore(), Continuations: continuation.NewMemoryStore(),
	}
}
