package workspace

import (
	"context"
	"testing"

	"github.com/anurgosw/agentic-moe/execution"
)

type testSandboxDriver struct{ capabilities SandboxCapabilities }

func (testSandboxDriver) Name() string { return "test" }
func (d testSandboxDriver) Capabilities(context.Context) (SandboxCapabilities, error) {
	return d.capabilities, nil
}
func (testSandboxDriver) Start(_ context.Context, request SandboxRequest) (SandboxHandle, error) {
	return SandboxHandle{ID: request.ID, Driver: "test", EnvironmentDigest: request.Environment.Digest}, nil
}
func (testSandboxDriver) Stop(context.Context, SandboxHandle) error   { return nil }
func (testSandboxDriver) Health(context.Context, SandboxHandle) error { return nil }

func TestSandboxRuntimeFailsClosedOnUnenforcedLimits(t *testing.T) {
	environment, err := (Environment{Version: 1, Placement: PlacementContainer, WorkspaceLeaseID: "lease", ImageDigest: "sha256:image", Network: NetworkNone, Resources: ResourceLimits{MemoryBytes: 1024}}).Seal()
	if err != nil {
		t.Fatal(err)
	}
	request := SandboxRequest{
		ID: "box", Identity: execution.Identity{PrincipalID: "u", SessionID: "s", RunID: "r"}, Executable: "/bin/tool", WorkingDir: "/work",
		Environment: environment,
	}
	runtime := SandboxRuntime{Driver: testSandboxDriver{capabilities: SandboxCapabilities{Placements: []Placement{PlacementContainer}, NetworkPolicies: []NetworkPolicy{NetworkNone}}}}
	if _, err := runtime.Start(context.Background(), request); err == nil {
		t.Fatal("unenforced memory limit accepted")
	}
	runtime.Driver = testSandboxDriver{capabilities: SandboxCapabilities{Placements: []Placement{PlacementContainer}, NetworkPolicies: []NetworkPolicy{NetworkNone}, EnforcesMemory: true, EnforcesFilesystem: true}}
	if _, err := runtime.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}
