package linkproposal

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
)

func TestParseLLMResponse(t *testing.T) {
	raw := `{"proposals":[{"to_memory_id":72,"link_type":"related","confidence":0.85,"reason":"shared src_ip"}]}`
	allowed := map[int64]struct{}{72: {}}
	got, err := ParseLLMResponse(raw, 71, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LinkType != "related" || got[0].ToMemoryID != 72 {
		t.Fatalf("unexpected proposals: %+v", got)
	}
}

func TestParseLLMResponse_FiltersInvalid(t *testing.T) {
	raw := `{"proposals":[
		{"to_memory_id":71,"link_type":"related","confidence":0.5,"reason":"self"},
		{"to_memory_id":99,"link_type":"related","confidence":0.5,"reason":"not allowed"},
		{"to_memory_id":72,"link_type":"nonsense","confidence":0.5,"reason":"bad type"},
		{"to_memory_id":72,"link_type":"causal","confidence":0.5,"reason":""}
	]}`
	allowed := map[int64]struct{}{72: {}}
	got, err := ParseLLMResponse(raw, 71, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestBuildUserMessage(t *testing.T) {
	msg := BuildUserMessage(
		&model.MemoryEntry{ID: 1, Path: "root.a", Content: "alert"},
		&extraction.Record{Status: "ok", Slots: map[string]interface{}{"src_ip": "10.0.0.1"}},
		[]Candidate{{MemoryID: 2, Path: "root.b", Content: "other"}},
	)
	if msg == "" || !contains(msg, "Source memory id=1") || !contains(msg, "id=2") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
