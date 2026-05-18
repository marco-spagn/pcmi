package handler

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/event"
)

// Extra coverage for parseEventTypes and eventAllowed beyond what
// events_handler_test.go and handler_extra_test.go already cover.

func TestParseEventTypesWhitespaceOnly(t *testing.T) {
	if got := parseEventTypes("   "); got != nil {
		t.Fatalf("whitespace-only filter must return nil, got %v", got)
	}
}

func TestParseEventTypesDropsEmptyEntries(t *testing.T) {
	// trailing comma + double comma should not produce empty keys.
	got := parseEventTypes("memory.stored,,, ,knowledge.distilled,")
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct entries, got %d: %v", len(got), got)
	}
	if _, ok := got["memory.stored"]; !ok {
		t.Error(`expected "memory.stored" in allow-set`)
	}
	if _, ok := got["knowledge.distilled"]; !ok {
		t.Error(`expected "knowledge.distilled" in allow-set`)
	}
}

func TestParseEventTypesDeduplicates(t *testing.T) {
	got := parseEventTypes("memory.stored, memory.stored,memory.stored")
	if len(got) != 1 {
		t.Fatalf("expected dedup to 1, got %d: %v", len(got), got)
	}
}

func TestEventAllowedNoPayloadTenantField(t *testing.T) {
	// When the payload has no tenant_id, the event is allowed (defensive default).
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{}}
	if !eventAllowed(evt, nil, "tenant-a") {
		t.Fatal("payload without tenant_id should pass when no filter is set")
	}
}

func TestEventAllowedPayloadTenantNotString(t *testing.T) {
	// tenant_id present but wrong type → treated as "not set" → allowed.
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{"tenant_id": 12345}}
	if !eventAllowed(evt, nil, "tenant-a") {
		t.Fatal("non-string tenant_id should not block delivery")
	}
}

func TestEventAllowedFilterAndTenantBothMatch(t *testing.T) {
	allowed := parseEventTypes("memory.stored,knowledge.distilled")
	evt := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"tenant_id": "tenant-a"},
	}
	if !eventAllowed(evt, allowed, "tenant-a") {
		t.Fatal("matching filter + matching tenant should be allowed")
	}
}

func TestEventAllowedFilterPassButTenantFails(t *testing.T) {
	allowed := parseEventTypes("memory.stored")
	evt := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"tenant_id": "other"},
	}
	if eventAllowed(evt, allowed, "tenant-a") {
		t.Fatal("tenant mismatch must override filter match")
	}
}
