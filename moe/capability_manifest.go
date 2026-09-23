package moe

import (
	"github.com/anurgosw/agentic-moe/execution"
	"github.com/anurgosw/agentic-moe/policy"
	"github.com/tmc/langchaingo/llms"
)

// CapabilityResolverFromCatalog makes the model-visible schema and execution
// dispatcher consume the same immutable catalog and policy snapshots.
func CapabilityResolverFromCatalog(catalog execution.CatalogSnapshot, snapshot policy.Snapshot) CapabilityResolver {
	return func(toolSet string) []ToolBinding {
		capabilities := catalog.Capabilities(toolSet, snapshot)
		out := make([]ToolBinding, 0, len(capabilities))
		for _, capability := range capabilities {
			out = append(out, ToolBinding{
				Tool: llms.Tool{Type: "function", Function: &llms.FunctionDefinition{
					Name: capability.Ref.Name, Description: capability.Description, Parameters: capability.Schema,
				}},
				Ref: capability.Ref, SchemaDigest: capability.SchemaDigest,
			})
		}
		return out
	}
}
