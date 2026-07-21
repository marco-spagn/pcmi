package entityalias_test

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/entityalias"
)

func TestParseLLMResponse_Valid(t *testing.T) {
	raw := `{"proposals":[{"alias_key":"cozy bear","target_entity_id":"aaa-bbb","confidence":0.88,"reason":"same actor"}]}`
	allowed := map[string]struct{}{"aaa-bbb": {}}
	out, err := entityalias.ParseLLMResponse(raw, "source-id", allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].AliasKey != "cozy bear" {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestParseLLMResponse_RejectsUnknownTarget(t *testing.T) {
	raw := `{"proposals":[{"alias_key":"x","target_entity_id":"unknown","confidence":0.5,"reason":"nope"}]}`
	out, err := entityalias.ParseLLMResponse(raw, "source-id", map[string]struct{}{"aaa": {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty, got %+v", out)
	}
}
