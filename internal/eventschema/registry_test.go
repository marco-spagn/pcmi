package eventschema

import "testing"

func TestValidateAgentStepCompleted(t *testing.T) {
	if err := Validate("agent.step.completed", map[string]interface{}{"step": "plan"}); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
	if err := Validate("agent.step.completed", map[string]interface{}{}); err == nil {
		t.Fatal("expected missing step error")
	}
}

func TestValidateUnknownTypeAllowed(t *testing.T) {
	if err := Validate("custom.event.v2", map[string]interface{}{}); err != nil {
		t.Fatalf("unknown types should pass: %v", err)
	}
}

func TestValidateMemoryStored_missingRequiredField(t *testing.T) {
	if err := Validate("memory.stored", map[string]interface{}{
		"id": 1, "tenant_id": "t",
	}); err == nil {
		t.Fatal("expected missing path/version")
	}
}

func TestValidateAgentStepFailed(t *testing.T) {
	if err := Validate("agent.step.failed", map[string]interface{}{
		"step": "x", "error": "boom",
	}); err != nil {
		t.Fatal(err)
	}
	if err := Validate("agent.step.failed", map[string]interface{}{"step": "x"}); err == nil {
		t.Fatal("expected missing error")
	}
}

func TestListNonEmpty(t *testing.T) {
	if len(List()) < 5 {
		t.Fatalf("expected several schemas, got %d", len(List()))
	}
}
