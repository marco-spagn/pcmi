package extraction

import "testing"

func TestPromoteEntities(t *testing.T) {
	profile := &Profile{
		ProfileID: "soc.siem.v1",
		Version:   1,
		RequiredSlots: []SlotDef{
			{Name: "src_ip", Type: "ip", Nullable: true},
			{Name: "dst_host", Type: "hostname", Nullable: true},
		},
		EntityPromotion: map[string]EntityPromotion{
			"src_ip":   {VertexLabel: "IPAddress", Normalize: "trim"},
			"dst_host": {VertexLabel: "Asset", Normalize: "lower"},
		},
	}
	rec := &Record{
		Status:     "ok",
		Confidence: 0.9,
		Slots: map[string]interface{}{
			"src_ip":   "10.0.4.22",
			"dst_host": "MAIL-684",
		},
	}
	got := PromoteEntities(profile, rec)
	if len(got) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(got))
	}
	byKind := map[string]string{}
	for _, e := range got {
		byKind[e.Kind] = e.Key
	}
	if byKind["IPAddress"] != "10.0.4.22" {
		t.Fatalf("unexpected IP promotion: %+v", byKind)
	}
	if byKind["Asset"] != "mail-684" {
		t.Fatalf("expected lower-cased asset, got %+v", byKind)
	}
}

func TestPromoteEntities_SkipsNullAndFailed(t *testing.T) {
	profile := &Profile{
		EntityPromotion: map[string]EntityPromotion{
			"src_ip": {VertexLabel: "IPAddress", Normalize: "trim"},
		},
	}
	rec := &Record{
		Status: "validation_failed",
		Slots:  map[string]interface{}{"src_ip": "10.0.0.1"},
	}
	if len(PromoteEntities(profile, rec)) != 0 {
		t.Fatal("failed extraction must not promote")
	}
	rec.Status = "ok"
	rec.Slots["src_ip"] = nil
	if len(PromoteEntities(profile, rec)) != 0 {
		t.Fatal("null slot must not promote")
	}
}

func TestNormalizeEntityKey(t *testing.T) {
	cases := []struct {
		in, rule, want string
	}{
		{"  Foo  ", "trim", "Foo"},
		{"MAIL-01", "lower", "mail-01"},
		{"camp-1", "upper", "CAMP-1"},
		{"  ", "trim", ""},
	}
	for _, tc := range cases {
		if got := NormalizeEntityKey(tc.in, tc.rule); got != tc.want {
			t.Fatalf("NormalizeEntityKey(%q, %q) = %q, want %q", tc.in, tc.rule, got, tc.want)
		}
	}
}
