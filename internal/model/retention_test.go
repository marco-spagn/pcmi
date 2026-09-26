package model

import (
	"strings"
	"testing"
)

func ip(v int) *int { return &v }

func TestRetentionPolicyRequest_Normalize(t *testing.T) {
	ok := []RetentionPolicyRequest{
		{PathPrefix: " finance.ledger ", SupersededRetentionDays: ip(2555)},
		{PathPrefix: "", MaxAgeDays: ip(1)},
		{PathPrefix: "a", SupersededRetentionDays: ip(36500), MaxAgeDays: ip(90)},
	}
	for _, r := range ok {
		if err := r.Normalize(); err != nil {
			t.Fatalf("%+v: %v", r, err)
		}
	}
	if r := (RetentionPolicyRequest{PathPrefix: " x.y ", MaxAgeDays: ip(3)}); r.Normalize() == nil && r.PathPrefix != "x.y" {
		t.Fatal("prefix not trimmed")
	}
	bad := map[string]RetentionPolicyRequest{
		"no rule":        {PathPrefix: "a"},
		"bad prefix":     {PathPrefix: "a..b", MaxAgeDays: ip(1)},
		"zero days":      {PathPrefix: "a", MaxAgeDays: ip(0)},
		"too many days":  {PathPrefix: "a", SupersededRetentionDays: ip(36501)},
		"long desc":      {PathPrefix: "a", MaxAgeDays: ip(1), Description: strings.Repeat("x", 501)},
		"bad chars":      {PathPrefix: "a-b", MaxAgeDays: ip(1)},
		"negative years": {PathPrefix: "a", SupersededRetentionDays: ip(-5)},
	}
	for name, r := range bad {
		if err := r.Normalize(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestEraseRequest_Normalize(t *testing.T) {
	r := EraseRequest{PathPrefix: " users.alice ", Reason: " dsr-42 "}
	if err := r.Normalize(); err != nil || r.PathPrefix != "users.alice" || r.Reason != "dsr-42" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	for name, r := range map[string]EraseRequest{
		"empty prefix": {PathPrefix: "  "},
		"bad prefix":   {PathPrefix: "users.*"},
		"long reason":  {PathPrefix: "a", Reason: strings.Repeat("r", 501)},
	} {
		if err := r.Normalize(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
