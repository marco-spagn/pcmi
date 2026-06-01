package event

const (
	// PayloadKeyStreamID is set on published payloads when using Redis Streams (XADD entry ID).
	PayloadKeyStreamID = "stream_id"

	EventAgentStepCompleted = "agent.step.completed"
	EventToolCallExecuted   = "tool.call.executed"
	EventWorkflowFinished   = "workflow.finished"
	EventReasoningGenerated = "reasoning.generated"
	EventMemoryStored           = "memory.stored"
	EventMemoryUpdated          = "memory.updated"
	EventKnowledgeDistilled     = "knowledge.distilled"
	EventMemoryRefineRequested  = "memory.refine.requested"
	EventContradictionDetected  = "contradiction.detected"
)

type UniversalEvent struct {
	TenantID      string                 `json:"tenant_id"`
	EventType     string                 `json:"event_type"`
	Timestamp     string                 `json:"timestamp"`
	AgentID       string                 `json:"agent_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Payload       map[string]interface{} `json:"payload"`
}
