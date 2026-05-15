package handler

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/event"
)

func TestParseEventTypes(t *testing.T) {
	if parseEventTypes("") != nil {
		t.Fatal("empty filter should be nil (allow all)")
	}
	allowed := parseEventTypes("memory.stored, knowledge.distilled")
	if len(allowed) != 2 {
		t.Fatalf("expected 2 types, got %d", len(allowed))
	}
	if _, ok := allowed[event.EventMemoryStored]; !ok {
		t.Fatal("missing memory.stored")
	}
}

func TestEventAllowedTenantFilter(t *testing.T) {
	evt := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"tenant_id": "aaa"},
	}
	if !eventAllowed(evt, nil, "aaa") {
		t.Fatal("expected same tenant to pass")
	}
	if eventAllowed(evt, nil, "bbb") {
		t.Fatal("expected different tenant to be filtered")
	}
}
