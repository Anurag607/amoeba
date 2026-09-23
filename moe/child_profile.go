package moe

// ChildProfile makes child isolation explicit. Its zero value is the secure
// default: no transcript, memory, persistence, mutation, approval, or network
// inheritance.
type ChildProfile struct {
	InheritTranscript bool     `json:"inherit_transcript"`
	AllowPersistence  bool     `json:"allow_persistence"`
	AllowMemory       bool     `json:"allow_memory"`
	AllowMutation     bool     `json:"allow_mutation"`
	AllowApprovals    bool     `json:"allow_approvals"`
	AllowNetwork      bool     `json:"allow_network"`
	WorkspaceLease    string   `json:"workspace_lease,omitempty"`
	ContextSources    []string `json:"context_sources,omitempty"`
	OutputTokenBudget int      `json:"output_token_budget,omitempty"`
}

func (p ChildProfile) clone() ChildProfile {
	p.ContextSources = append([]string(nil), p.ContextSources...)
	return p
}
