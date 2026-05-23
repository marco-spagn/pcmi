package grpcserver

import (
	"testing"
	"time"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
)

func TestClampAdminLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want int
	}{
		{0, 50},
		{-1, 50},
		{10, 10},
		{200, 200},
		{500, 200},
	}
	for _, tc := range cases {
		if got := clampAdminLimit(int32(tc.in)); got != tc.want {
			t.Fatalf("clampAdminLimit(%d)=%d want %d", tc.in, got, tc.want)
		}
	}
}

func TestAPIKeySummaryFromMap(t *testing.T) {
	t.Parallel()
	created := time.Unix(1700000000, 0).UTC()
	exp := time.Unix(1800000000, 0).UTC()
	sum, err := apiKeySummaryFromMap(map[string]interface{}{
		"id":         "kid",
		"name":       "n",
		"role":       "admin",
		"is_active":  true,
		"created_at": created,
		"expires_at": exp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Id != "kid" || sum.Role != "admin" || !sum.IsActive {
		t.Fatalf("unexpected %+v", sum)
	}
	if sum.CreatedAt == nil || sum.ExpiresAt != exp.Format(time.RFC3339) {
		t.Fatalf("timestamps: created=%v expires=%q", sum.CreatedAt, sum.ExpiresAt)
	}
}

func TestAPIKeySummaryFromMap_numericID(t *testing.T) {
	t.Parallel()
	sum, err := apiKeySummaryFromMap(map[string]interface{}{
		"id":        42,
		"name":      "n",
		"role":      "user",
		"is_active": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Id != "42" {
		t.Fatalf("id=%q", sum.Id)
	}
}

func TestAPIKeyToProto_nil(t *testing.T) {
	t.Parallel()
	if apiKeyToProto(nil).Id != "" {
		t.Fatal("nil response should yield empty proto")
	}
}

func TestTenantToProto_withCreatedAt(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	out := tenantToProto(&model.TenantResponse{
		ID: "t", Slug: "s", Name: "n", CreatedAt: ts,
	})
	if out.CreatedAt == nil {
		t.Fatal("expected created_at on proto")
	}
}

// compile-time guard that pcmiv1 types are wired
func TestAPIKeySummaryProtoFields(t *testing.T) {
	t.Parallel()
	_ = &pcmiv1.APIKeySummary{TenantId: "t"}
}
