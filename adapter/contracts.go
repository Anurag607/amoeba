// Package adapter defines narrow host integration contracts for MCP servers
// and external agent harnesses. It contains no transport implementation.
package adapter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Anurag607/amoeba/execution"
	"github.com/Anurag607/amoeba/trajectory"
	"github.com/Anurag607/amoeba/workspace"
)

type Health string

const (
	HealthStarting Health = "starting"
	HealthReady    Health = "ready"
	HealthDegraded Health = "degraded"
	HealthStopped  Health = "stopped"
)

type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
	TransportSSE   Transport = "sse"
)

type AuthMode string

const (
	AuthNone  AuthMode = "none"
	AuthToken AuthMode = "token"
	AuthOAuth AuthMode = "oauth"
)

// ResumeMode names a concrete recovery mechanism rather than treating every
// harness Resume method as equivalent.
type ResumeMode string

const (
	ResumeNone            ResumeMode = ""
	ResumeToken           ResumeMode = "token"
	ResumeTranscript      ResumeMode = "transcript"
	ResumeExactCheckpoint ResumeMode = "exact_checkpoint"
)

// HarnessCapabilities are host-observed properties of a configured harness.
// A manifest pins these values so a resumed request cannot silently negotiate
// weaker recovery semantics.
type HarnessCapabilities struct {
	ResumeToken       bool `json:"resume_token"`
	TranscriptReplay  bool `json:"transcript_replay"`
	ExactCheckpoint   bool `json:"exact_checkpoint"`
	EventReplay       bool `json:"event_replay"`
	WorkspaceSnapshot bool `json:"workspace_snapshot"`
	IdempotentResume  bool `json:"idempotent_resume"`
}

func (c HarnessCapabilities) Supports(mode ResumeMode) bool {
	switch mode {
	case ResumeNone:
		return true
	case ResumeToken:
		return c.ResumeToken
	case ResumeTranscript:
		return c.TranscriptReplay
	case ResumeExactCheckpoint:
		return c.ExactCheckpoint
	default:
		return false
	}
}

type MCPConfig struct {
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	Transport        Transport     `json:"transport"`
	Endpoint         string        `json:"endpoint"`
	Auth             AuthMode      `json:"auth"`
	ConnectTimeout   time.Duration `json:"connect_timeout"`
	CallTimeout      time.Duration `json:"call_timeout"`
	ReconnectBackoff time.Duration `json:"reconnect_backoff"`
}

type MCPStatus struct {
	Health         Health    `json:"health"`
	CatalogVersion string    `json:"catalog_version,omitempty"`
	Authenticated  bool      `json:"authenticated"`
	LastError      string    `json:"last_error,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// MCPServer is the lifecycle surface required from a local or remote MCP
// transport. OAuth and transport details remain implementation-specific.
type MCPServer interface {
	Validate(MCPConfig) error
	Configure(context.Context, MCPConfig) error
	Start(context.Context) error
	Authenticate(context.Context) error
	RefreshAuth(context.Context) error
	Discover(context.Context) (MCPCatalog, error)
	WatchCatalog(context.Context) (<-chan MCPCatalog, error)
	CallTool(context.Context, MCPToolCall) (MCPToolResult, error)
	ReadResource(context.Context, MCPResourceRequest) (json.RawMessage, error)
	RenderPrompt(context.Context, MCPPromptRequest) (json.RawMessage, error)
	Health(context.Context) (Health, error)
	Status(context.Context) (MCPStatus, error)
	Stop(context.Context) error
}

type MCPCatalog struct {
	Version   string           `json:"version"`
	Tools     []MCPTool        `json:"tools,omitempty"`
	Resources []MCPResourceRef `json:"resources,omitempty"`
	Prompts   []MCPPromptRef   `json:"prompts,omitempty"`
}

type MCPTool struct {
	Ref          execution.ToolRef   `json:"ref"`
	SchemaDigest string              `json:"schema_digest"`
	Class        execution.ToolClass `json:"class"`
}

type MCPResourceRef struct {
	URI     string `json:"uri"`
	Version string `json:"version"`
}

type MCPPromptRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type MCPToolCall struct {
	Envelope execution.OperationEnvelope `json:"envelope"`
}

type MCPToolResult struct {
	Content       json.RawMessage `json:"content,omitempty"`
	Indeterminate bool            `json:"indeterminate,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	ResultHash    string          `json:"result_hash,omitempty"`
}

type MCPResourceRequest struct {
	Identity execution.Identity `json:"identity"`
	Resource MCPResourceRef     `json:"resource"`
}

type MCPPromptRequest struct {
	Identity  execution.Identity `json:"identity"`
	Prompt    MCPPromptRef       `json:"prompt"`
	Arguments json.RawMessage    `json:"arguments,omitempty"`
}

type HarnessRequest struct {
	Identity          execution.Identity    `json:"identity"`
	InputID           string                `json:"input_id"`
	Prompt            string                `json:"prompt"`
	Model             string                `json:"model"`
	Environment       workspace.Environment `json:"environment"`
	ChildRunID        string                `json:"child_run_id"`
	PolicyVersion     string                `json:"policy_version"`
	CatalogVersion    string                `json:"catalog_version"`
	ContextDigest     string                `json:"context_digest,omitempty"`
	OutputTokenBudget int                   `json:"output_token_budget,omitempty"`
	RequiredResume    ResumeMode            `json:"required_resume,omitempty"`
	ContinuationID    string                `json:"continuation_id,omitempty"`
	CheckpointHash    string                `json:"checkpoint_hash,omitempty"`
	ResumeToken       string                `json:"resume_token,omitempty"`
	Deadline          time.Time             `json:"deadline,omitempty"`
	Harness           HarnessManifest       `json:"harness"`
}

type HarnessResult struct {
	Status         string          `json:"status"`
	Final          string          `json:"final,omitempty"`
	ArtifactRef    string          `json:"artifact_ref,omitempty"`
	InputTokens    int             `json:"input_tokens,omitempty"`
	OutputTokens   int             `json:"output_tokens,omitempty"`
	ResumeMode     ResumeMode      `json:"resume_mode,omitempty"`
	ResumeToken    string          `json:"resume_token,omitempty"`
	ContinuationID string          `json:"continuation_id,omitempty"`
	CheckpointHash string          `json:"checkpoint_hash,omitempty"`
	ContextDigest  string          `json:"context_digest,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type HarnessConfig struct {
	Name       string            `json:"name"`
	Executable string            `json:"executable"`
	Arguments  []string          `json:"arguments,omitempty"`
	AllowEnv   []string          `json:"allow_env,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type HarnessStatus struct {
	Name       string    `json:"name"`
	Version    string    `json:"version,omitempty"`
	Health     Health    `json:"health"`
	Configured bool      `json:"configured"`
	ActiveRuns int       `json:"active_runs"`
	LastError  string    `json:"last_error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type HarnessManifest struct {
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	ConfigDigest string              `json:"config_digest"`
	Health       Health              `json:"health"`
	Capabilities HarnessCapabilities `json:"capabilities"`
}

// HarnessCapabilityReporter lets an adapter expose configured recovery
// guarantees without expanding the execution interface for legacy harnesses.
type HarnessCapabilityReporter interface {
	Capabilities(context.Context) (HarnessCapabilities, error)
}

// AgentHarness normalizes lifecycle, cancellation, events, and terminal
// outcome for one explicitly supported external agent runtime.
type AgentHarness interface {
	Name() string
	Validate(HarnessConfig) error
	Configure(context.Context, HarnessConfig) error
	Version(context.Context) (string, error)
	Run(context.Context, HarnessRequest, func(trajectory.Event) error) (HarnessResult, error)
	Resume(context.Context, HarnessRequest, func(trajectory.Event) error) (HarnessResult, error)
	Cancel(context.Context, string) error
	Status(context.Context) (HarnessStatus, error)
	Health(context.Context) (Health, error)
}
