package moe

import (
	"context"
	"fmt"
	"strings"
)

// ContextFragment is untrusted source material selected for a child. Content
// must be supplied to the child as data, never concatenated into its trusted
// task or system instructions.
type ContextFragment struct {
	Ref     string `json:"ref"`
	Content string `json:"content"`
}

// ContextResolver resolves a model-selected opaque reference to canonical
// host-owned content. It must reject unknown or unauthorized references.
type ContextResolver func(ctx context.Context, ref string) (ContextFragment, error)

func resolveContextRefs(ctx context.Context, refs []string, resolver ContextResolver) ([]ContextFragment, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("selected_context requires a host context resolver")
	}
	frames := make([]ContextFragment, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, rawRef := range refs {
		ref := strings.TrimSpace(rawRef)
		if ref == "" {
			return nil, fmt.Errorf("selected_context references must be non-empty")
		}
		if _, duplicate := seen[ref]; duplicate {
			return nil, fmt.Errorf("duplicate selected_context ref %q", ref)
		}
		seen[ref] = struct{}{}
		frame, err := resolver(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve selected context %q: %w", ref, err)
		}
		if strings.TrimSpace(frame.Ref) != ref || strings.TrimSpace(frame.Content) == "" {
			return nil, fmt.Errorf("resolve selected context %q: resolver returned mismatched or empty frame", ref)
		}
		frames = append(frames, frame)
	}
	return frames, nil
}
