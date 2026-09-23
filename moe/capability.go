package moe

import "github.com/tmc/langchaingo/llms"

// CapabilityCeiling is the exact set of tool names a delegated plan may
// expose. Its zero value denies every tool, so callers must opt in to each
// delegated capability.
type CapabilityCeiling struct {
	allowed      map[string]struct{}
	unrestricted bool
}

// NewCapabilityCeiling constructs a fail-closed ceiling for the supplied
// tool names.
func NewCapabilityCeiling(names ...string) CapabilityCeiling {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	return CapabilityCeiling{allowed: allowed}
}

// UnrestrictedCapabilityCeiling explicitly permits every resolved tool. It
// is intended for trusted/manual expert runs, not model-directed delegation.
func UnrestrictedCapabilityCeiling() CapabilityCeiling {
	return CapabilityCeiling{unrestricted: true}
}

// Allows reports whether a named tool is inside the ceiling.
func (c CapabilityCeiling) Allows(name string) bool {
	if c.unrestricted {
		return true
	}
	_, ok := c.allowed[name]
	return ok
}

func (c CapabilityCeiling) filter(tools []llms.Tool) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]llms.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool.Function != nil && c.Allows(tool.Function.Name) {
			out = append(out, tool)
		}
	}
	return out
}

func toolNames(tools []llms.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Function != nil && tool.Function.Name != "" {
			names = append(names, tool.Function.Name)
		}
	}
	return names
}
