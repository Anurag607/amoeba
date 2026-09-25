package evaluation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

const maxOutputSchemaBytes = 64 << 10
const maxContractOutputBytes = 1 << 20

// OutputContractKind identifies a provider-neutral response constraint.
type OutputContractKind string

const (
	// OutputJSONSchema requires one JSON object matching Schema.
	OutputJSONSchema OutputContractKind = "json_schema"
)

// OutputContract is a bounded, provider-neutral response constraint. The zero
// value requests ordinary text generation.
type OutputContract struct {
	Kind   OutputContractKind `json:"kind"`
	Schema json.RawMessage    `json:"schema,omitempty"`
}

// NewJSONSchemaContract validates and copies an object-shaped JSON Schema.
func NewJSONSchemaContract(schema []byte) (OutputContract, error) {
	contract := OutputContract{Kind: OutputJSONSchema, Schema: append(json.RawMessage(nil), schema...)}
	if err := contract.Validate(); err != nil {
		return OutputContract{}, err
	}
	return contract, nil
}

// Validate rejects ambiguous, oversized, or non-object response contracts.
func (c OutputContract) Validate() error {
	if c.Kind == "" {
		if len(bytes.TrimSpace(c.Schema)) != 0 {
			return fmt.Errorf("output contract: schema requires a kind")
		}
		return nil
	}
	if c.Kind != OutputJSONSchema {
		return fmt.Errorf("output contract: unsupported kind %q", c.Kind)
	}
	if len(c.Schema) == 0 || len(c.Schema) > maxOutputSchemaBytes {
		return fmt.Errorf("output contract: schema must be between 1 and %d bytes", maxOutputSchemaBytes)
	}
	var schema map[string]any
	if err := json.Unmarshal(c.Schema, &schema); err != nil {
		return fmt.Errorf("output contract: invalid JSON Schema: %w", err)
	}
	if schema == nil || schema["type"] != "object" {
		return fmt.Errorf("output contract: JSON Schema type must be object")
	}
	return validateSchemaDefinition(schema, "$", 0)
}

// ValidateOutput performs strict local validation for the JSON Schema subset
// used by evaluator contracts. Provider constraints improve generation, but
// local validation remains authoritative when a provider violates a schema.
func (c OutputContract) ValidateOutput(output string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Kind == "" {
		return nil
	}
	if len(output) > maxContractOutputBytes {
		return fmt.Errorf("output contract: response exceeds %d bytes", maxContractOutputBytes)
	}
	var schema map[string]any
	if err := json.Unmarshal(c.Schema, &schema); err != nil {
		return fmt.Errorf("output contract: decode schema: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("output contract: decode response: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return err
	}
	return validateSchemaValue(schema, value, "$", 0)
}

func validateSchemaDefinition(schema map[string]any, path string, depth int) error {
	if depth > 32 {
		return fmt.Errorf("output contract: schema nesting exceeds 32 levels")
	}
	for keyword := range schema {
		if !supportedSchemaKeyword(keyword) {
			return fmt.Errorf("output contract: %s uses unsupported keyword %q", path, keyword)
		}
	}
	schemaType, ok := schema["type"].(string)
	if !ok || !supportedSchemaType(schemaType) {
		return fmt.Errorf("output contract: %s requires a supported type", path)
	}
	if enum, exists := schema["enum"]; exists {
		values, ok := enum.([]any)
		if !ok || len(values) == 0 {
			return fmt.Errorf("output contract: %s enum must be a non-empty array", path)
		}
		for _, value := range values {
			if !valueMatchesType(schemaType, value) {
				return fmt.Errorf("output contract: %s enum value does not match type %q", path, schemaType)
			}
		}
	}
	switch schemaType {
	case "object":
		if hasAnyKeyword(schema, "items", "minLength") {
			return fmt.Errorf("output contract: %s has a keyword incompatible with object", path)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || len(properties) == 0 {
			return fmt.Errorf("output contract: %s requires object properties", path)
		}
		for name, property := range properties {
			propertySchema, ok := property.(map[string]any)
			if !ok {
				return fmt.Errorf("output contract: %s.%s must be a schema object", path, name)
			}
			if err := validateSchemaDefinition(propertySchema, path+"."+name, depth+1); err != nil {
				return err
			}
		}
		if required, exists := schema["required"]; exists {
			values, ok := required.([]any)
			if !ok {
				return fmt.Errorf("output contract: %s required must be an array", path)
			}
			seen := make(map[string]struct{}, len(values))
			for _, value := range values {
				name, ok := value.(string)
				if !ok || name == "" {
					return fmt.Errorf("output contract: %s required names must be non-empty strings", path)
				}
				if _, duplicate := seen[name]; duplicate {
					return fmt.Errorf("output contract: %s required property %q is duplicated", path, name)
				}
				if _, exists := properties[name]; !exists {
					return fmt.Errorf("output contract: %s required property %q is not defined", path, name)
				}
				seen[name] = struct{}{}
			}
		}
		if additional, exists := schema["additionalProperties"]; exists {
			if _, ok := additional.(bool); !ok {
				return fmt.Errorf("output contract: %s additionalProperties must be boolean", path)
			}
		}
	case "array":
		if hasAnyKeyword(schema, "properties", "required", "additionalProperties", "minLength") {
			return fmt.Errorf("output contract: %s has a keyword incompatible with array", path)
		}
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return fmt.Errorf("output contract: %s requires an items schema", path)
		}
		return validateSchemaDefinition(items, path+"[]", depth+1)
	case "string":
		if hasAnyKeyword(schema, "properties", "required", "additionalProperties", "items") {
			return fmt.Errorf("output contract: %s has a keyword incompatible with string", path)
		}
		if minimum, exists := schema["minLength"]; exists {
			value, ok := minimum.(float64)
			if !ok || value < 0 || math.Trunc(value) != value {
				return fmt.Errorf("output contract: %s minLength must be a non-negative integer", path)
			}
		}
	default:
		if hasAnyKeyword(schema, "properties", "required", "additionalProperties", "items", "minLength") {
			return fmt.Errorf("output contract: %s has a keyword incompatible with %s", path, schemaType)
		}
	}
	return nil
}

func supportedSchemaKeyword(keyword string) bool {
	switch keyword {
	case "$schema", "$id", "title", "description", "default", "examples", "type", "properties", "required", "additionalProperties", "items", "enum", "minLength":
		return true
	default:
		return false
	}
}

func supportedSchemaType(schemaType string) bool {
	switch schemaType {
	case "object", "array", "string", "integer", "number", "boolean":
		return true
	default:
		return false
	}
}

func hasAnyKeyword(schema map[string]any, keywords ...string) bool {
	for _, keyword := range keywords {
		if _, exists := schema[keyword]; exists {
			return true
		}
	}
	return false
}

func valueMatchesType(schemaType string, value any) bool {
	switch schemaType {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		number, ok := numberValue(value)
		return ok && math.Trunc(number) == number
	case "number":
		_, ok := numberValue(value)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return false
	}
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("output contract: response contains multiple JSON values")
		}
		return fmt.Errorf("output contract: trailing response data: %w", err)
	}
	return nil
}

func validateSchemaValue(schema map[string]any, value any, path string, depth int) error {
	if depth > 32 {
		return fmt.Errorf("output contract: schema nesting exceeds 32 levels")
	}
	if enum, ok := schema["enum"].([]any); ok && !enumContains(enum, value) {
		return fmt.Errorf("output contract: %s is not an allowed value", path)
	}
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("output contract: %s must be an object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, item := range required {
				name, _ := item.(string)
				if _, exists := object[name]; !exists {
					return fmt.Errorf("output contract: %s.%s is required", path, name)
				}
			}
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			for name := range object {
				if _, exists := properties[name]; !exists {
					return fmt.Errorf("output contract: %s.%s is not allowed", path, name)
				}
			}
		}
		for name, property := range properties {
			propertySchema, ok := property.(map[string]any)
			if !ok {
				continue
			}
			if item, exists := object[name]; exists {
				if err := validateSchemaValue(propertySchema, item, path+"."+name, depth+1); err != nil {
					return err
				}
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("output contract: %s must be an array", path)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for index, item := range items {
			if err := validateSchemaValue(itemSchema, item, fmt.Sprintf("%s[%d]", path, index), depth+1); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("output contract: %s must be a string", path)
		}
		if minimum, ok := schema["minLength"].(float64); ok && float64(utf8.RuneCountInString(text)) < minimum {
			return fmt.Errorf("output contract: %s is shorter than minLength", path)
		}
	case "integer":
		number, ok := numberValue(value)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("output contract: %s must be an integer", path)
		}
	case "number":
		if _, ok := numberValue(value); !ok {
			return fmt.Errorf("output contract: %s must be a number", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("output contract: %s must be a boolean", path)
		}
	}
	return nil
}

func enumContains(values []any, candidate any) bool {
	if candidateNumber, ok := numberValue(candidate); ok {
		for _, value := range values {
			if enumNumber, ok := numberValue(value); ok && enumNumber == candidateNumber {
				return true
			}
		}
		return false
	}
	wanted, err := json.Marshal(candidate)
	if err != nil {
		return false
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err == nil && bytes.Equal(encoded, wanted) {
			return true
		}
	}
	return false
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case json.Number:
		value, err := number.Float64()
		return value, err == nil
	default:
		return 0, false
	}
}

func (c OutputContract) clone() OutputContract {
	c.Schema = append(json.RawMessage(nil), c.Schema...)
	return c
}

func mustJSONSchemaContract(schema string) OutputContract {
	contract, err := NewJSONSchemaContract([]byte(schema))
	if err != nil {
		panic(err)
	}
	return contract
}
