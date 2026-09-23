package moe

// EventType is the kind of observability event emitted during planning.
type EventType string

const (
	EventTypeRouting             EventType = "routing"
	EventTypeExpert              EventType = "expert_activated"
	EventTypeSecondary           EventType = "secondary_consulted"
	EventTypeSynthesis           EventType = "synthesis_triggered"
	EventTypeFallback            EventType = "fallback"
	EventTypeDelegation          EventType = "delegation"
	EventTypeCentral             EventType = "central_dispatch"
	EventTypeDelegationStarted   EventType = "delegation_started"
	EventTypeDelegationCompleted EventType = "delegation_completed"
	EventTypeDelegationFailed    EventType = "delegation_failed"
	EventTypeDelegationPressure  EventType = "delegation_pressure"
)

// Event is the observability record emitted by the orchestrator. Consumers
// typically forward these to the UI / tracing layer.
type Event struct {
	Type         EventType `json:"type"`
	ExpertID     ExpertID  `json:"expert_id,omitempty"`
	ExpertName   string    `json:"expert_name,omitempty"`
	Confidence   float64   `json:"confidence,omitempty"`
	Skills       []string  `json:"skills,omitempty"`
	Tier         string    `json:"tier,omitempty"`
	Message      string    `json:"message"`
	Reasoning    string    `json:"reasoning,omitempty"`
	FromExpert   ExpertID  `json:"from_expert,omitempty"`
	ToExpert     ExpertID  `json:"to_expert,omitempty"`
	NodeID       string    `json:"node_id,omitempty"`
	ParentNodeID string    `json:"parent_node_id,omitempty"`
	Depth        int       `json:"depth,omitempty"`
	Tokens       int       `json:"tokens,omitempty"`
	DurationMS   int64     `json:"duration_ms,omitempty"`
}

// EventEmitter is a callback the orchestrator calls when planning produces
// observability events. nil is safe (drops events).
type EventEmitter func(Event)
