package runtimekit

import "encoding/json"

// Schema returns the JSON Schema for the current configuration version.
func Schema() map[string]any {
	expert := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"id", "name"},
		"properties": map[string]any{
			"id": expertIDSchema(), "name": nonEmptyString(), "description": stringSchema(),
			"keywords": stringArray(), "negative_keywords": stringArray(), "prompt": stringSchema(),
			"default_tier": map[string]any{"type": "string", "enum": []string{"fast", "balanced", "strong"}},
			"priority":     integerSchema(), "can_synthesize": boolSchema(),
			"max_iterations": nonNegativeInt(), "max_tool_calls": nonNegativeInt(),
		},
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://agentic-moe.dev/schema/config-v1.json",
		"title":   "agentic-moe configuration", "type": "object", "additionalProperties": false,
		"required": []string{"version"},
		"properties": map[string]any{
			"version": map[string]any{"const": ConfigVersion},
			"runtime": objectSchema(map[string]any{"max_iterations": positiveInt(), "max_tool_calls": positiveInt(), "response_token_budget": positiveInt(), "max_injected_skill_tokens": positiveInt()}),
			"models":  objectSchema(map[string]any{"fast": stringSchema(), "balanced": stringSchema(), "strong": stringSchema()}),
			"experts": map[string]any{"type": "array", "minItems": 1, "items": expert},
			"ollama": objectSchema(map[string]any{
				"enabled": boolSchema(), "base_url": map[string]any{"type": "string", "format": "uri"},
				"allow_remote": boolSchema(), "timeout": durationSchema(), "max_body_bytes": positiveInt(),
				"context_window": positiveInt(), "max_parallel": positiveInt(),
			}),
			"http": objectSchema(map[string]any{
				"address": nonEmptyString(), "bearer_token_env": stringSchema(), "allowed_hosts": stringArray(), "allowed_origins": stringArray(),
				"max_body_bytes": positiveInt(), "max_concurrent": positiveInt(),
				"request_timeout": durationSchema(), "shutdown_grace": durationSchema(),
			}),
		},
	}
}

// SchemaJSON renders the schema deterministically enough for CLI output.
func SchemaJSON() ([]byte, error) { return json.MarshalIndent(Schema(), "", "  ") }

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
}

func positiveInt() map[string]any    { return map[string]any{"type": "integer", "minimum": 1} }
func stringSchema() map[string]any   { return map[string]any{"type": "string"} }
func nonEmptyString() map[string]any { return map[string]any{"type": "string", "minLength": 1} }
func expertIDSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9_-]{0,63}$`}
}
func stringArray() map[string]any    { return map[string]any{"type": "array", "items": stringSchema()} }
func integerSchema() map[string]any  { return map[string]any{"type": "integer"} }
func nonNegativeInt() map[string]any { return map[string]any{"type": "integer", "minimum": 0} }
func boolSchema() map[string]any     { return map[string]any{"type": "boolean"} }
func durationSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^[0-9]+(?:\.[0-9]+)?(?:ns|us|µs|ms|s|m|h)$`}
}
