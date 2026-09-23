// Package mcpserver exposes runtimekit through the official MCP protocol SDK.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anurgosw/agentic-moe/internal/buildinfo"
	"github.com/anurgosw/agentic-moe/runtimekit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server owns the protocol surface for one immutable runtime composition.
type Server struct {
	runtime *runtimekit.Runtime
	mcp     *mcp.Server
}

// New registers stable tools, resources, and prompts.
func New(runtime *runtimekit.Runtime) (*Server, error) {
	if runtime == nil {
		return nil, fmt.Errorf("mcp server: runtime is required")
	}
	server := &Server{runtime: runtime}
	server.mcp = mcp.NewServer(&mcp.Implementation{Name: "agentic-moe", Version: buildinfo.Version}, nil)
	server.registerTools()
	server.registerResources()
	server.registerPrompts()
	return server, nil
}

// ProtocolServer returns the underlying official MCP server for custom transports.
func (s *Server) ProtocolServer() *mcp.Server { return s.mcp }

// RunStdio serves MCP over stdin/stdout. Callers must keep logs on stderr.
func (s *Server) RunStdio(ctx context.Context) error {
	if err := s.mcp.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serve mcp stdio: %w", err)
	}
	return nil
}

type planInput struct {
	Query   string                  `json:"query" jsonschema:"required,Task to route and plan"`
	Routing runtimekit.RoutingInput `json:"routing,omitempty" jsonschema:"Optional routing context"`
}

type expertPlanInput struct {
	ExpertID string `json:"expert_id" jsonschema:"required,Registered expert identifier"`
}

type validateInput struct {
	Config string `json:"config" jsonschema:"required,YAML or JSON configuration text"`
}

type validationOutput struct {
	Valid   bool   `json:"valid"`
	Version int    `json:"version"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) registerTools() {
	readOnly := true
	closedWorld := false
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true, OpenWorldHint: &closedWorld}
	mcp.AddTool(s.mcp, &mcp.Tool{Name: "plan_task", Title: "Plan task", Description: "Route a task to an expert and return a bounded host-executable plan.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input planInput) (*mcp.CallToolResult, runtimekit.PlanView, error) {
			out, err := s.runtime.Plan(ctx, input.Query, input.Routing)
			return nil, out, err
		})
	mcp.AddTool(s.mcp, &mcp.Tool{Name: "plan_for_expert", Title: "Plan for expert", Description: "Build a plan for an explicitly selected expert with tools denied by default.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input expertPlanInput) (*mcp.CallToolResult, runtimekit.PlanView, error) {
			out, err := s.runtime.PlanForExpert(ctx, input.ExpertID)
			return nil, out, err
		})
	mcp.AddTool(s.mcp, &mcp.Tool{Name: "validate_config", Title: "Validate configuration", Description: "Strictly validate agentic-moe YAML or JSON without persisting it.", Annotations: annotations},
		func(_ context.Context, _ *mcp.CallToolRequest, input validateInput) (*mcp.CallToolResult, validationOutput, error) {
			cfg, err := runtimekit.Parse([]byte(input.Config), func(string) string { return "" })
			if err != nil {
				return nil, validationOutput{Valid: false, Version: runtimekit.ConfigVersion, Error: "configuration is invalid"}, nil
			}
			return nil, validationOutput{Valid: true, Version: cfg.Version}, nil
		})
}

func (s *Server) registerResources() {
	resources := []struct {
		uri, name, description string
		value                  func(context.Context) any
	}{
		{"agentic-moe://manifest", "manifest", "Framework experts, providers, transports, and side-effect posture.", func(context.Context) any { return s.runtime.Manifest() }},
		{"agentic-moe://experts", "experts", "Configured expert catalog.", func(context.Context) any { return s.runtime.Manifest().Experts }},
		{"agentic-moe://providers", "providers", "Configured provider source names; reading does not invoke models.", func(context.Context) any { return s.runtime.Manifest().Providers }},
		{"agentic-moe://health", "health", "Current runtime and provider discovery health.", func(ctx context.Context) any { return s.runtime.Health(ctx) }},
	}
	for _, item := range resources {
		item := item
		s.mcp.AddResource(&mcp.Resource{URI: item.uri, Name: item.name, Description: item.description, MIMEType: "application/json"},
			func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				body, err := json.MarshalIndent(item.value(ctx), "", "  ")
				if err != nil {
					return nil, fmt.Errorf("marshal resource %s: %w", item.name, err)
				}
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: item.uri, MIMEType: "application/json", Text: string(body)}}}, nil
			})
	}
}

func (s *Server) registerPrompts() {
	s.mcp.AddPrompt(&mcp.Prompt{
		Name: "orchestrate_task", Title: "Orchestrate a task", Description: "Ask a host model to use agentic-moe planning before execution.",
		Arguments: []*mcp.PromptArgument{{Name: "task", Description: "Task to plan", Required: true}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		task := req.Params.Arguments["task"]
		if task == "" {
			return nil, fmt.Errorf("prompt: task is required")
		}
		message := "Call plan_task for the following task. Treat the returned plan as guidance and keep all credentials and side effects under host control.\n\nTask:\n" + task
		return &mcp.GetPromptResult{Description: "Plan before executing", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: message}}}}, nil
	})
}
