package extraction_test

import (
	"encoding/json"
	"testing"

	"github.com/marco-spagn/pcmi/internal/extraction"
)

func TestValidateProfile_ok(t *testing.T) {
	raw := []byte(`{
		"profile_id":"soc.siem.v1",
		"version":1,
		"required_slots":[
			{"name":"severity","type":"enum","values":["P1","P2"]},
			{"name":"src_ip","type":"ip","nullable":true}
		]
	}`)
	p, err := extraction.ParseProfile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.ProfileID != "soc.siem.v1" {
		t.Fatalf("profile_id=%q", p.ProfileID)
	}
}

func TestValidateSlots_allKeysRequired(t *testing.T) {
	p := &extraction.Profile{
		ProfileID: "generic.record.v1",
		Version:   1,
		RequiredSlots: []extraction.SlotDef{
			{Name: "subject", Type: "string", Nullable: false},
			{Name: "primary_actor", Type: "string", Nullable: true},
		},
	}
	slots, err := extraction.ValidateSlots(p, map[string]interface{}{
		"subject":       "ticket opened",
		"primary_actor": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if slots["primary_actor"] != nil {
		t.Fatalf("expected null primary_actor, got %v", slots["primary_actor"])
	}
}

func TestParseLLMResponse_invalidIP(t *testing.T) {
	p := &extraction.Profile{
		ProfileID: "soc.siem.v1",
		Version:   1,
		RequiredSlots: []extraction.SlotDef{
			{Name: "src_ip", Type: "ip", Nullable: true},
		},
	}
	_, err := extraction.ParseLLMResponse(`{"confidence":0.9,"slots":{"src_ip":"not-an-ip"}}`, p)
	if err == nil {
		t.Fatal("expected invalid IP error")
	}
}

func TestParseLLMResponse_ok(t *testing.T) {
	p := &extraction.Profile{
		ProfileID: "soc.siem.v1",
		Version:   1,
		RequiredSlots: []extraction.SlotDef{
			{Name: "severity", Type: "enum", Values: []string{"P1", "P2"}, Nullable: false},
			{Name: "src_ip", Type: "ip", Nullable: true},
		},
	}
	rec, err := extraction.ParseLLMResponse(`{
		"confidence": 0.82,
		"slots": {"severity":"P2","src_ip":"10.0.0.1"},
		"evidence_spans":[{"slot":"src_ip","quote":"10.0.0.1"}]
	}`, p)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Confidence != 0.82 || rec.Slots["severity"] != "P2" {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestRecordFromMetadata_roundTrip(t *testing.T) {
	rec := &extraction.Record{
		ProfileID:      "soc.siem.v1",
		ProfileVersion: 1,
		MemoryID:       42,
		MemoryVersion:  2,
		Confidence:     0.5,
		Slots:          map[string]interface{}{"severity": "P1"},
		Status:         "ok",
	}
	meta := extraction.RecordToMetadataMap(rec)
	got, ok := extraction.RecordFromMetadata(meta)
	if !ok || got.ProfileID != "soc.siem.v1" || got.Slots["severity"] != "P1" {
		t.Fatalf("round trip failed: ok=%v got=%+v meta=%s", ok, got, mustJSON(meta))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestShouldSkipPath(t *testing.T) {
	if !extraction.ShouldSkipPath("root.test.distilled.summary") {
		t.Fatal("expected skip distilled path")
	}
	if extraction.ShouldSkipPath("root.soc.inc_1") {
		t.Fatal("expected soc path")
	}
}
