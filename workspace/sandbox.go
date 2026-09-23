package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anurgosw/agentic-moe/execution"
)

type SandboxCapabilities struct {
	Placements         []Placement     `json:"placements"`
	NetworkPolicies    []NetworkPolicy `json:"network_policies"`
	EnforcesCPU        bool            `json:"enforces_cpu"`
	EnforcesMemory     bool            `json:"enforces_memory"`
	EnforcesProcesses  bool            `json:"enforces_processes"`
	EnforcesFilesystem bool            `json:"enforces_filesystem"`
	EnforcesSecrets    bool            `json:"enforces_secrets"`
}

type SandboxRequest struct {
	ID          string             `json:"id"`
	Identity    execution.Identity `json:"identity"`
	Environment Environment        `json:"environment"`
	Executable  string             `json:"executable"`
	Arguments   []string           `json:"arguments,omitempty"`
	WorkingDir  string             `json:"working_dir"`
	Deadline    time.Time          `json:"deadline,omitempty"`
}

type SandboxHandle struct {
	ID                string    `json:"id"`
	Driver            string    `json:"driver"`
	EnvironmentDigest string    `json:"environment_digest"`
	StartedAt         time.Time `json:"started_at"`
}

// SandboxDriver is an enforcement backend, not merely a process launcher.
// Capabilities are checked before Start so unsupported restrictions fail shut.
type SandboxDriver interface {
	Name() string
	Capabilities(context.Context) (SandboxCapabilities, error)
	Start(context.Context, SandboxRequest) (SandboxHandle, error)
	Stop(context.Context, SandboxHandle) error
	Health(context.Context, SandboxHandle) error
}

type SandboxRuntime struct{ Driver SandboxDriver }

func (r SandboxRuntime) Start(ctx context.Context, request SandboxRequest) (SandboxHandle, error) {
	if r.Driver == nil {
		return SandboxHandle{}, fmt.Errorf("sandbox driver is required")
	}
	if request.ID == "" || request.Identity.Validate() != nil || strings.TrimSpace(request.Executable) == "" || strings.TrimSpace(request.WorkingDir) == "" {
		return SandboxHandle{}, fmt.Errorf("sandbox request requires ID, identity, executable, and working directory")
	}
	if err := request.Environment.Validate(); err != nil {
		return SandboxHandle{}, err
	}
	capabilities, err := r.Driver.Capabilities(ctx)
	if err != nil {
		return SandboxHandle{}, fmt.Errorf("sandbox capabilities: %w", err)
	}
	if !containsPlacement(capabilities.Placements, request.Environment.Placement) || !containsNetwork(capabilities.NetworkPolicies, request.Environment.Network) {
		return SandboxHandle{}, fmt.Errorf("sandbox driver cannot enforce requested placement or network policy")
	}
	if !capabilities.EnforcesFilesystem || (len(request.Environment.SecretBindings) > 0 && !capabilities.EnforcesSecrets) {
		return SandboxHandle{}, fmt.Errorf("sandbox driver cannot enforce filesystem or secret bindings")
	}
	limits := request.Environment.Resources
	if (limits.CPUMillis > 0 && !capabilities.EnforcesCPU) || (limits.MemoryBytes > 0 && !capabilities.EnforcesMemory) || (limits.ProcessCount > 0 && !capabilities.EnforcesProcesses) {
		return SandboxHandle{}, fmt.Errorf("sandbox driver cannot enforce requested resource limits")
	}
	handle, err := r.Driver.Start(ctx, request)
	if err != nil {
		return SandboxHandle{}, fmt.Errorf("start sandbox %q: %w", r.Driver.Name(), err)
	}
	if handle.ID == "" || handle.Driver != r.Driver.Name() || handle.EnvironmentDigest != request.Environment.Digest {
		_ = r.Driver.Stop(context.WithoutCancel(ctx), handle)
		return SandboxHandle{}, fmt.Errorf("sandbox driver returned an unbound handle")
	}
	return handle, nil
}

func containsPlacement(values []Placement, want Placement) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsNetwork(values []NetworkPolicy, want NetworkPolicy) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
