package grpcserver

import (
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/event"
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
)

// Event gRPC streaming uses these helpers for filtering — worth a direct check.
func TestEventGRPCHelpers(t *testing.T) {
	if parseEventTypesGRPC("") != nil {
		t.Fatal("empty input should yield nil map")
	}
	m := parseEventTypesGRPC(" a , b ")
	if len(m) != 2 {
		t.Fatalf("got %v", m)
	}
	allowed := map[string]struct{}{"x": {}}
	if eventAllowedGRPC(event.Event{Type: "y", Payload: map[string]any{}}, allowed, "") {
		t.Fatal("type filter")
	}
	if !eventAllowedGRPC(event.Event{Type: "x", Payload: map[string]any{}}, allowed, "") {
		t.Fatal("type allowed")
	}
	evt := event.Event{Type: "t", Payload: map[string]any{"tenant_id": "A"}}
	if eventAllowedGRPC(evt, nil, "B") {
		t.Fatal("tenant mismatch")
	}
	if !eventAllowedGRPC(evt, nil, "A") {
		t.Fatal("tenant match")
	}
}

// Mappings used by several unary RPCs: invalid payload / rollback args are common 400s.
func TestOperationalProtoConversions(t *testing.T) {
	link, err := createLinkProtoToModel(&pcmiv1.CreateLinkRequest{
		FromPath: "a", ToPath: "b", MetadataJson: `{"x":1}`, LinkType: " cites ",
	})
	if err != nil || link.LinkType != "cites" {
		t.Fatalf("link %#v err=%v", link, err)
	}
	if v, ok := link.Metadata["x"].(float64); !ok || v != 1 {
		t.Fatalf("metadata %#v", link.Metadata)
	}

	if _, err = ingestEventProtoToModel(&pcmiv1.IngestEventRequest{PayloadJson: "{"}); err == nil {
		t.Fatal("expected JSON error")
	}
	ing, err := ingestEventProtoToModel(&pcmiv1.IngestEventRequest{
		EventType: "e", AgentId: "ag", PayloadJson: `{"z":true}`,
	})
	if err != nil || ing.EventType != "e" || ing.AgentID != "ag" {
		t.Fatalf("ingest %#v err=%v", ing, err)
	}

	if _, err = rollbackProtoToModel(&pcmiv1.RollbackRequest{Path: ""}); err == nil {
		t.Fatal("expected path error")
	}
	if _, err = rollbackProtoToModel(&pcmiv1.RollbackRequest{Path: "p", AsOfRfc3339: "bad"}); err == nil {
		t.Fatal("expected as_of error")
	}
	rb, err := rollbackProtoToModel(&pcmiv1.RollbackRequest{
		Path: "root.p", Version: 2, AsOfRfc3339: "2020-01-02T15:04:05Z",
	})
	if err != nil || rb.Path != "root.p" || rb.Version == nil || *rb.Version != 2 {
		t.Fatalf("rollback %#v err=%v", rb, err)
	}
	sid := int64(7)
	pb := rollbackToProto(&model.RollbackResponse{ID: 1, Version: 3, SupersededID: &sid})
	if pb.GetSupersededId() != 7 {
		t.Fatal(pb)
	}

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if ep := ingestEventToProto(&model.IngestEventResponse{
		ID: "i", EventType: "t", Status: "ok", Timestamp: ts,
	}); ep.GetId() != "i" || ep.GetTimestampRfc3339() == "" {
		t.Fatal(ep)
	}
}
