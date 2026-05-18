package event

import (
	"encoding/json"
	"testing"
)

// TestEventTypeConstantsStable guards against accidental rename of the event
// type identifiers — these are part of the public contract for webhooks and
// the gRPC event stream and may not change without a major version bump.
func TestEventTypeConstantsStable(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{EventAgentStepCompleted, "agent.step.completed"},
		{EventToolCallExecuted, "tool.call.executed"},
		{EventWorkflowFinished, "workflow.finished"},
		{EventReasoningGenerated, "reasoning.generated"},
		{EventMemoryStored, "memory.stored"},
		{EventMemoryUpdated, "memory.updated"},
		{EventKnowledgeDistilled, "knowledge.distilled"},
		{EventMemoryRefineRequested, "memory.refine.requested"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("event type changed: got %q want %q", c.got, c.want)
		}
	}
}

func TestUniversalEventRoundTripJSON(t *testing.T) {
	in := UniversalEvent{
		TenantID:      "tenant-a",
		EventType:     EventMemoryStored,
		Timestamp:     "2026-01-01T00:00:00Z",
		AgentID:       "agent-x",
		CorrelationID: "corr-1",
		Payload:       map[string]interface{}{"key": "value", "n": 42.0},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out UniversalEvent
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.TenantID != in.TenantID || out.EventType != in.EventType ||
		out.Timestamp != in.Timestamp || out.AgentID != in.AgentID ||
		out.CorrelationID != in.CorrelationID {
		t.Fatalf("round-trip mismatch:\nin=%+v\nout=%+v", in, out)
	}
	if out.Payload["key"] != "value" {
		t.Errorf("payload.key lost: got %v", out.Payload["key"])
	}
}

func TestUniversalEventOmitsEmptyOptionals(t *testing.T) {
	// AgentID and CorrelationID are tagged ,omitempty — verify they don't
	// appear in the marshalled JSON when unset.
	evt := UniversalEvent{
		TenantID:  "tenant-a",
		EventType: EventMemoryStored,
		Timestamp: "2026-01-01T00:00:00Z",
		Payload:   map[string]interface{}{},
	}
	raw, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	for _, key := range []string{"agent_id", "correlation_id"} {
		if contains(s, key) {
			t.Errorf("expected %q to be omitted: %s", key, s)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
