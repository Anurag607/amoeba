package moe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anurgosw/agentic-moe/agent"
	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/skills"

	"github.com/tmc/langchaingo/llms"
)

// ConsultRequest is the parsed argument shape for consult_expert.
type ConsultRequest struct {
	ExpertID        ExpertID          `json:"expert_id"`
	Question        string            `json:"question"`
	SelectedContext []string          `json:"selected_context,omitempty"`
	ContextFrames   []ContextFragment `json:"-"`
}

// ConsultFn is the consumer-supplied callback that handles a consult call.
// Implementations typically pull the expert's skill bodies and run a single
// LLM completion (no full sub-agent loop). Compare with DelegateFn which
// runs a full sub-loop.
type ConsultFn func(ctx context.Context, req ConsultRequest, expert *ExpertDefinition) (string, error)

// ConsultOptions provides canonical context resolution for advisory calls.
type ConsultOptions struct {
	ResolveContext ContextResolver
}

// ConsultTool returns the consult_expert tool definition. The description
// enumerates available experts so the central agent knows valid IDs.
func ConsultTool(reg *Registry) llms.Tool {
	return ConsultToolFor(reg.All())
}

func ConsultToolFor(experts []*ExpertDefinition) llms.Tool {
	parts := make([]string, 0, len(experts))
	for _, e := range experts {
		parts = append(parts, fmt.Sprintf("%s — %s", e.ID, e.Description))
	}
	desc := "Consult a specialist expert for a focused question. The expert " +
		"answers using its domain knowledge (no tool execution). Available experts: " +
		strings.Join(parts, "; ")
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "consult_expert",
			Description: desc,
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"expert_id": { "type": "string", "description": "ID of the expert to consult." },
					"question":  { "type": "string", "description": "Focused question for the expert." },
					"selected_context": { "type": "array", "description": "Opaque host-owned context references resolved by the server.", "items": { "type": "string", "minLength": 1 }, "uniqueItems": true }
				},
				"required": ["expert_id", "question"],
				"additionalProperties": false
			}`),
		},
	}
}

// RegisterConsult attaches the consult_expert callable on a BaseAgent.
func RegisterConsult(base *agent.BaseAgent, reg *Registry, fn ConsultFn) error {
	return RegisterConsultWithOptions(base, reg, fn, ConsultOptions{})
}

// RegisterConsultWithOptions attaches consult_expert with canonical context
// resolution. Calls without selected context need no resolver.
func RegisterConsultWithOptions(base *agent.BaseAgent, reg *Registry, fn ConsultFn, opts ConsultOptions) error {
	definition := ConsultTool(reg)
	schema := toolSchema(definition)
	return base.Register(execution.ToolSpec{
		Ref:         execution.ToolRef{Name: "consult_expert", Version: "v1"},
		Description: definition.Function.Description, Schema: schema,
		SchemaDigest: toolSchemaDigest(definition),
		Class:        execution.ToolClass{Category: "delegation"},
		Validate: func(arguments string) error {
			var req ConsultRequest
			if err := decodeStrictJSON(arguments, &req); err != nil {
				return err
			}
			if req.ExpertID == "" || strings.TrimSpace(req.Question) == "" {
				return fmt.Errorf("expert_id and question are required")
			}
			return nil
		},
		ResolveResource: func(arguments string) (string, error) {
			var req ConsultRequest
			if err := decodeStrictJSON(arguments, &req); err != nil {
				return "", err
			}
			return "expert:" + string(req.ExpertID), nil
		},
		ResolveAuthorizations: func(arguments string) ([]execution.AuthorizationTarget, error) {
			var req ConsultRequest
			if err := decodeStrictJSON(arguments, &req); err != nil {
				return nil, err
			}
			return []execution.AuthorizationTarget{{Action: "expert." + string(req.ExpertID)}}, nil
		},
		Execute: func(ctx context.Context, arguments string) (string, error) {
			var req ConsultRequest
			if err := decodeStrictJSON(arguments, &req); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if req.ExpertID == "" || strings.TrimSpace(req.Question) == "" {
				return "", fmt.Errorf("expert_id and question are required")
			}
			frames, err := resolveContextRefs(ctx, req.SelectedContext, opts.ResolveContext)
			if err != nil {
				return "", err
			}
			req.ContextFrames = frames
			expert, ok := reg.Get(req.ExpertID)
			if !ok {
				return fmt.Sprintf("Unknown expert %q.", req.ExpertID), nil
			}
			return fn(ctx, req, expert)
		},
	})
}

func toolSchemaDigest(tool llms.Tool) string {
	if tool.Function == nil {
		return ""
	}
	sum := sha256.Sum256(toolSchema(tool))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func toolSchema(tool llms.Tool) json.RawMessage {
	if tool.Function == nil {
		return nil
	}
	switch value := tool.Function.Parameters.(type) {
	case json.RawMessage:
		return append(json.RawMessage(nil), value...)
	case []byte:
		return append(json.RawMessage(nil), value...)
	case string:
		return json.RawMessage(value)
	default:
		payload, _ := json.Marshal(value)
		return payload
	}
}

// CentralPromptAugmentation returns the system-prompt block describing the
// available experts to a "central" agent (one allowed to call consult_expert
// and delegate_to_expert).
func CentralPromptAugmentation(reg *Registry, skillReg *skills.Registry) string {
	return CentralPromptAugmentationFor(reg.All(), skillReg)
}

func CentralPromptAugmentationFor(experts []*ExpertDefinition, skillReg *skills.Registry) string {
	if len(experts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n## EXPERT NETWORK\n\n")
	sb.WriteString("You are the central coordinator. Other experts are available via the ")
	sb.WriteString("`consult_expert` (advisory, no tool execution) and `delegate_to_expert` ")
	sb.WriteString("(full sub-investigation with tool execution) tools.\n\n")
	for _, e := range experts {
		fmt.Fprintf(&sb, "- **%s** (`%s`): %s\n", e.Name, e.ID, e.Description)
		if skillReg != nil && len(e.SkillIDs) > 0 {
			fmt.Fprintf(&sb, "  Skills: %s\n", strings.Join(e.SkillIDs, ", "))
		}
	}
	sb.WriteString("\nPrefer consult_expert for quick advisory questions and ")
	sb.WriteString("delegate_to_expert when the sub-task needs its own tool calls.\n")
	return sb.String()
}
