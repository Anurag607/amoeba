package evaluation

import (
	"bytes"
	"strings"
	"testing"
)

func TestJSONSchemaContractValidatesAndCopies(t *testing.T) {
	source := []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)
	contract, err := NewJSONSchemaContract(source)
	if err != nil {
		t.Fatal(err)
	}
	source[0] = '['
	if contract.Kind != OutputJSONSchema || !bytes.HasPrefix(contract.Schema, []byte(`{"type"`)) {
		t.Fatalf("contract = %+v", contract)
	}
	clone := contract.clone()
	clone.Schema[0] = '['
	if contract.Schema[0] != '{' {
		t.Fatal("clone aliases contract schema")
	}
}

func TestOutputContractRejectsMalformedSchemas(t *testing.T) {
	valid := []byte(`{"type":"object","properties":{"value":{"type":"string"}}}`)
	for name, contract := range map[string]OutputContract{
		"schema without kind": {Schema: valid},
		"unsupported kind":    {Kind: "grammar", Schema: valid},
		"invalid JSON":        {Kind: OutputJSONSchema, Schema: []byte(`{`)},
		"non-object":          {Kind: OutputJSONSchema, Schema: []byte(`{"type":"array","properties":{"value":{}}}`)},
		"missing properties":  {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object"}`)},
		"invalid required":    {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{}},"required":"value"}`)},
		"unknown required":    {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{}},"required":["missing"]}`)},
		"duplicate required":  {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value","value"]}`)},
		"nested non-schema":   {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":"string"}}`)},
		"unsupported type":    {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{"type":"null"}}}`)},
		"unsupported keyword": {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{"type":"string","pattern":"x"}}}`)},
		"invalid additional":  {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{"type":"string"}},"additionalProperties":"false"}`)},
		"array without items": {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{"type":"array"}}}`)},
		"invalid min length":  {Kind: OutputJSONSchema, Schema: []byte(`{"type":"object","properties":{"value":{"type":"string","minLength":-1}}}`)},
		"oversized":           {Kind: OutputJSONSchema, Schema: []byte(strings.Repeat("x", maxOutputSchemaBytes+1))},
	} {
		t.Run(name, func(t *testing.T) {
			if err := contract.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
	if err := (OutputContract{}).Validate(); err != nil {
		t.Fatalf("zero contract: %v", err)
	}
}

func TestOutputContractStrictlyValidatesResponses(t *testing.T) {
	contract := mustJSONSchemaContract(`{"type":"object","properties":{"name":{"type":"string","minLength":1},"count":{"type":"integer","enum":[2]},"tags":{"type":"array","items":{"type":"string"}}},"required":["name","count","tags"],"additionalProperties":false}`)
	if err := contract.ValidateOutput(`{"name":"ok","count":2,"tags":["one","two"]}`); err != nil {
		t.Fatalf("valid response: %v", err)
	}
	for name, output := range map[string]string{
		"invalid JSON":        `not-json`,
		"multiple values":     `{"name":"ok","count":2,"tags":[]} {}`,
		"missing required":    `{"name":"ok","count":2}`,
		"additional property": `{"name":"ok","count":2,"tags":[],"extra":true}`,
		"wrong type":          `{"name":"ok","count":"2","tags":[]}`,
		"wrong enum":          `{"name":"ok","count":3,"tags":[]}`,
		"short string":        `{"name":"","count":2,"tags":[]}`,
		"wrong array item":    `{"name":"ok","count":2,"tags":[1]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := contract.ValidateOutput(output); err == nil {
				t.Fatal("ValidateOutput() error = nil")
			}
		})
	}
}

func FuzzOutputContractValidateOutput(f *testing.F) {
	contract := mustJSONSchemaContract(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)
	f.Add(`{"value":"ok"}`)
	f.Add(`{"value":1}`)
	f.Add(`not-json`)
	f.Fuzz(func(t *testing.T, output string) {
		_ = contract.ValidateOutput(output)
	})
}
