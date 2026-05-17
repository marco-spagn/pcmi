package service

import (
	"strings"
	"testing"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestEventServiceIngestMissingEventType(t *testing.T) {
	svc := NewEventService(nil)
	_, err := svc.Ingest(t.Context(), &model.IngestEventRequest{
		Payload: map[string]interface{}{"key": "val"},
	}, "tid")
	if err == nil || !strings.Contains(err.Error(), "event_type") {
		t.Fatalf("expected event_type error, got %v", err)
	}
}

func TestEventServiceIngestValidationFails(t *testing.T) {
	svc := NewEventService(nil)
	// "memory.stored" requires id, tenant_id, path, version
	_, err := svc.Ingest(t.Context(), &model.IngestEventRequest{
		EventType: "memory.stored",
		Payload:   map[string]interface{}{"incomplete": true},
	}, "tid")
	if err == nil {
		t.Fatal("expected schema validation error for incomplete memory.stored payload")
	}
}

func TestEventServiceIngestWhitespaceEventType(t *testing.T) {
	svc := NewEventService(nil)
	_, err := svc.Ingest(t.Context(), &model.IngestEventRequest{
		EventType: "   ",
		Payload:   map[string]interface{}{},
	}, "tid")
	if err == nil || !strings.Contains(err.Error(), "event_type") {
		t.Fatalf("expected event_type error for whitespace, got %v", err)
	}
}


