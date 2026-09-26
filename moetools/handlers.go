// Package moetools provides the load_skill and load_reference tool
// definitions and handlers. They let the LLM dynamically pull skill bodies
// and reference documents from a skills.Registry during the agentic loop,
// instead of stuffing every skill into the base system prompt.
package moetools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Anurag607/amoeba/agent"
	"github.com/Anurag607/amoeba/execution"
	"github.com/Anurag607/amoeba/skills"

	"github.com/tmc/langchaingo/llms"
)

// Tool names.
const (
	NameLoadSkill     = "load_skill"
	NameLoadReference = "load_reference"
)

// DescribeAvailableSkills enumerates a registry into a one-line catalog
// suitable for inclusion in the load_skill tool description.
func DescribeAvailableSkills(reg *skills.Registry) string {
	all := reg.All()
	if len(all) == 0 {
		return "(no skills registered)"
	}
	parts := make([]string, 0, len(all))
	for _, s := range all {
		parts = append(parts, fmt.Sprintf("%s — %s", s.ID, s.Description))
	}
	return strings.Join(parts, "; ")
}

// LoadSkillTool returns the langchaingo tool definition for load_skill,
// with a description that enumerates `reg`'s current skill catalog so the
// model knows which IDs are valid.
func LoadSkillTool(reg *skills.Registry) llms.Tool {
	desc := "Load the full body of a domain-knowledge skill into context. " +
		"Use this when you need investigation strategy or domain heuristics for a topic. " +
		"Available skills: " + DescribeAvailableSkills(reg)
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        NameLoadSkill,
			Description: desc,
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"skill_id": {
						"type": "string",
						"description": "The skill ID to load (see catalog in tool description)."
					}
				},
				"required": ["skill_id"]
			}`),
		},
	}
}

// LoadReferenceTool returns the langchaingo tool definition for load_reference.
func LoadReferenceTool() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name: NameLoadReference,
			Description: "Load a named reference document attached to a previously-loaded skill. " +
				"Skills enumerate their references in the response of load_skill.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"skill_id":  { "type": "string", "description": "ID of the parent skill." },
					"reference": { "type": "string", "description": "Reference name (without .md)." }
				},
				"required": ["skill_id", "reference"]
			}`),
		},
	}
}

// LoadSkillHandler implements the load_skill tool against the given registry.
func LoadSkillHandler(reg *skills.Registry, arguments string) (string, error) {
	var args struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.SkillID == "" {
		return "", fmt.Errorf("skill_id is required")
	}
	skill, ok := reg.Get(args.SkillID)
	if !ok {
		return fmt.Sprintf(
			"Unknown skill %q. Available: %s",
			args.SkillID,
			strings.Join(reg.AllIDs(), ", "),
		), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Skill: %s\n\n", skill.Name)
	if skill.Description != "" {
		fmt.Fprintf(&sb, "_%s_\n\n", skill.Description)
	}
	sb.WriteString(skill.Content)
	if refs := skill.ReferenceNames(); len(refs) > 0 {
		sb.WriteString("\n\n### Available References\n")
		sb.WriteString("Use `load_reference` to pull any of these for deeper context:\n")
		for _, r := range refs {
			fmt.Fprintf(&sb, "- skill_id=%q reference=%q\n", skill.ID, r)
		}
	}
	return sb.String(), nil
}

// LoadReferenceHandler implements the load_reference tool against the given
// registry.
func LoadReferenceHandler(reg *skills.Registry, arguments string) (string, error) {
	var args struct {
		SkillID   string `json:"skill_id"`
		Reference string `json:"reference"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.SkillID == "" || args.Reference == "" {
		return "", fmt.Errorf("both 'skill_id' and 'reference' are required")
	}
	content, ok := reg.GetReference(args.SkillID, args.Reference)
	if !ok {
		skill, exists := reg.Get(args.SkillID)
		if !exists {
			return fmt.Sprintf("Unknown skill %q.", args.SkillID), nil
		}
		return fmt.Sprintf(
			"Reference %q not found in skill %q. Available: %s",
			args.Reference, args.SkillID,
			strings.Join(skill.ReferenceNames(), ", "),
		), nil
	}
	return content, nil
}

// Register attaches the load_skill / load_reference callables onto a
// BaseAgent. Consumers typically call this on every agent so the catalog is
// available regardless of which expert is active.
func Register(base *agent.BaseAgent, reg *skills.Registry) error {
	loadSkill := LoadSkillTool(reg)
	loadSkillSchema := toolSchema(loadSkill)
	if err := base.Register(execution.ToolSpec{
		Ref: execution.ToolRef{Name: NameLoadSkill, Version: "v1"}, Description: loadSkill.Function.Description,
		Schema: loadSkillSchema, SchemaDigest: schemaDigest(loadSkill), Class: execution.ToolClass{Category: "skills", TrustedOutput: true},
		Validate: func(args string) error { return validateSkillArgs(args, false) },
		ResolveResource: func(args string) (string, error) {
			var value struct {
				SkillID string `json:"skill_id"`
			}
			if err := decodeStrict(args, &value); err != nil {
				return "", err
			}
			return "skill:" + value.SkillID, nil
		},
		ResolveAuthorizations: func(args string) ([]execution.AuthorizationTarget, error) {
			var value struct {
				SkillID string `json:"skill_id"`
			}
			if err := decodeStrict(args, &value); err != nil {
				return nil, err
			}
			return []execution.AuthorizationTarget{{Action: "skill." + value.SkillID}}, nil
		},
		Execute: func(_ context.Context, args string) (string, error) { return LoadSkillHandler(reg, args) },
	}); err != nil {
		return err
	}
	loadReference := LoadReferenceTool()
	loadReferenceSchema := toolSchema(loadReference)
	return base.Register(execution.ToolSpec{
		Ref: execution.ToolRef{Name: NameLoadReference, Version: "v1"}, Description: loadReference.Function.Description,
		Schema: loadReferenceSchema, SchemaDigest: schemaDigest(loadReference), Class: execution.ToolClass{Category: "skills", TrustedOutput: true},
		Validate: func(args string) error { return validateSkillArgs(args, true) },
		ResolveResource: func(args string) (string, error) {
			var value struct {
				SkillID   string `json:"skill_id"`
				Reference string `json:"reference"`
			}
			if err := decodeStrict(args, &value); err != nil {
				return "", err
			}
			return "skill:" + value.SkillID + "/" + value.Reference, nil
		},
		ResolveAuthorizations: func(args string) ([]execution.AuthorizationTarget, error) {
			var value struct {
				SkillID   string `json:"skill_id"`
				Reference string `json:"reference"`
			}
			if err := decodeStrict(args, &value); err != nil {
				return nil, err
			}
			return []execution.AuthorizationTarget{{Action: "skill." + value.SkillID}}, nil
		},
		Execute: func(_ context.Context, args string) (string, error) { return LoadReferenceHandler(reg, args) },
	})
}

func schemaDigest(tool llms.Tool) string {
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

func validateSkillArgs(arguments string, reference bool) error {
	var value struct {
		SkillID   string `json:"skill_id"`
		Reference string `json:"reference,omitempty"`
	}
	if err := decodeStrict(arguments, &value); err != nil {
		return err
	}
	if value.SkillID == "" || (reference && value.Reference == "") {
		return fmt.Errorf("required skill arguments are missing")
	}
	return nil
}

func decodeStrict(arguments string, value any) error {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

// Tools returns the slice of tool definitions to append to any chat tool set.
func Tools(reg *skills.Registry) []llms.Tool {
	return []llms.Tool{LoadSkillTool(reg), LoadReferenceTool()}
}
