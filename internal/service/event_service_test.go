package service

import (
	"strings"
	"testing"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestIngestRequiresEventType(t *testing.T) {
	svc := NewEventService(nil)
	_, err := svc.Ingest(t.Context(), &model.IngestEventRequest{Payload: map[string]interface{}{}}, "tenant")
	if err == nil || !strings.Contains(err.Error(), "event_type") {
		t.Fatalf("expected event_type error, got %v", err)
	}
}
