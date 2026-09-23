package runtimekit

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Migrate upgrades supported legacy configuration to the current schema.
// Versionless v0 files are the only legacy form and map directly to v1.
func Migrate(body []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parse config for migration: %w", err)
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("parse config for migration: empty document")
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parse config for migration: root must be an object")
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "version" {
			return body, nil
		}
	}
	mapping.Content = append([]*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"},
		{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"},
	}, mapping.Content...)
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("encode migrated config: %w", err)
	}
	return out.Bytes(), nil
}
