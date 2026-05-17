package eventschema

import "testing"

func TestLookupKnownSchema(t *testing.T) {
	s, ok := Lookup("memory.stored")
	if !ok {
		t.Fatal("expected memory.stored to be registered")
	}
	if s.EventType != "memory.stored" {
		t.Fatalf("unexpected event type: %s", s.EventType)
	}
}

func TestLookupUnknownSchema(t *testing.T) {
	_, ok := Lookup("not.a.real.event")
	if ok {
		t.Fatal("expected unknown event to return false")
	}
}

func TestValidateMemoryStoredAllRequired(t *testing.T) {
	payload := map[string]interface{}{
		"id":        1,
		"tenant_id": "tid",
		"path":      "root.x",
		"version":   2,
	}
	if err := Validate("memory.stored", payload); err != nil {
		t.Fatalf("expected valid payload to pass: %v", err)
	}
}

func TestValidateMemoryStoredMissingRequired(t *testing.T) {
	// 'path' is missing
	payload := map[string]interface{}{
		"id":        1,
		"tenant_id": "tid",
		"version":   1,
	}
	if err := Validate("memory.stored", payload); err == nil {
		t.Fatal("expected error for missing required field 'path'")
	}
}

func TestValidateKnowledgeDistilled(t *testing.T) {
	payload := map[string]interface{}{
		"tenant_id": "tid",
		"path":      "root.test",
	}
	if err := Validate("knowledge.distilled", payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateStrictUnknownField(t *testing.T) {
	// Strict schema: extra unknown fields are OK for now (non-strict enforcement)
	payload := map[string]interface{}{
		"id":        1,
		"tenant_id": "tid",
		"path":      "root.x",
		"version":   1,
		"extra":     "ignored",
	}
	if err := Validate("memory.stored", payload); err != nil {
		t.Fatalf("extra fields should not cause validation failure: %v", err)
	}
}

func TestListContainsExpectedTypes(t *testing.T) {
	schemas := List()
	found := map[string]bool{}
	for _, s := range schemas {
		found[s.EventType] = true
	}
	required := []string{
		"memory.stored",
		"memory.updated",
		"knowledge.distilled",
		"agent.step.completed",
	}
	for _, r := range required {
		if !found[r] {
			t.Errorf("expected schema %q to be registered", r)
		}
	}
}
