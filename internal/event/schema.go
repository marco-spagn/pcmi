package event

const (
	EventAgentStepCompleted = "agent.step.completed"
	EventToolCallExecuted   = "tool.call.executed"
	EventWorkflowFinished   = "workflow.finished"
	EventReasoningGenerated = "reasoning.generated"
	EventMemoryStored       = "memory.stored"
	EventMemoryUpdated      = "memory.updated"
	EventKnowledgeDistilled = "knowledge.distilled"
	EventMemoryRefineRequested = "memory.refine.requested"
)

type UniversalEvent struct {
	TenantID      string                 `json:"tenant_id"`
	EventType     string                 `json:"event_type"`
	Timestamp     string                 `json:"timestamp"`
	AgentID       string                 `json:"agent_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Payload       map[string]interface{} `json:"payload"`
}
