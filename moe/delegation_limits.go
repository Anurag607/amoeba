package moe

import "time"

// DelegationLimits bounds one root run, including every recursive child.
type DelegationLimits struct {
	MaxDepth               int
	MaxReasoningDepth      int
	MaxNodes               int
	MaxChildren            int
	MaxReasoningChildren   int
	MaxParallel            int
	MaxTotalTokens         int
	MaxNodeTokens          int
	RootFinalizationTokens int
	MaxWorkUnits           int
	MaxRequestTokens       int
	MaxArtifactBytes       int
	MaxTotalArtifactBytes  int
	MaxArtifactItems       int
	Timeout                time.Duration
}

// DefaultDelegationLimits returns conservative process-local limits.
func DefaultDelegationLimits() DelegationLimits {
	return DelegationLimits{
		MaxDepth: 3, MaxReasoningDepth: 2, MaxNodes: 12,
		MaxChildren: 3, MaxReasoningChildren: 2, MaxParallel: 2,
		MaxTotalTokens: 32768, MaxNodeTokens: 8192,
		RootFinalizationTokens: 4096, MaxWorkUnits: 32,
		MaxRequestTokens: 4096, MaxArtifactBytes: 4096,
		MaxTotalArtifactBytes: 65536, MaxArtifactItems: 32, Timeout: 2 * time.Minute,
	}
}

func normalizeDelegationLimits(in DelegationLimits) DelegationLimits {
	d := DefaultDelegationLimits()
	if in.MaxDepth > 0 {
		d.MaxDepth = in.MaxDepth
	}
	if in.MaxReasoningDepth > 0 {
		d.MaxReasoningDepth = in.MaxReasoningDepth
	}
	if in.MaxNodes > 0 {
		d.MaxNodes = in.MaxNodes
	}
	if in.MaxChildren > 0 {
		d.MaxChildren = in.MaxChildren
	}
	if in.MaxReasoningChildren > 0 {
		d.MaxReasoningChildren = in.MaxReasoningChildren
	}
	if in.MaxParallel > 0 {
		d.MaxParallel = in.MaxParallel
	}
	if in.MaxTotalTokens > 0 {
		d.MaxTotalTokens = in.MaxTotalTokens
	}
	if in.MaxNodeTokens > 0 {
		d.MaxNodeTokens = in.MaxNodeTokens
	}
	if in.RootFinalizationTokens > 0 {
		d.RootFinalizationTokens = in.RootFinalizationTokens
	} else if in.RootFinalizationTokens < 0 {
		d.RootFinalizationTokens = 0
	}
	if in.MaxWorkUnits > 0 {
		d.MaxWorkUnits = in.MaxWorkUnits
	}
	if in.MaxRequestTokens > 0 {
		d.MaxRequestTokens = in.MaxRequestTokens
	}
	if in.MaxArtifactBytes > 0 {
		d.MaxArtifactBytes = in.MaxArtifactBytes
	}
	if in.MaxTotalArtifactBytes > 0 {
		d.MaxTotalArtifactBytes = in.MaxTotalArtifactBytes
	}
	if in.MaxArtifactItems > 0 {
		d.MaxArtifactItems = in.MaxArtifactItems
	}
	if in.Timeout > 0 {
		d.Timeout = in.Timeout
	}
	if d.RootFinalizationTokens >= d.MaxTotalTokens {
		d.RootFinalizationTokens = d.MaxTotalTokens / 8
	}
	return d
}
