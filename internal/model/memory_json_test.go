package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStoreRequestJSONRoundTrip(t *testing.T) {
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	in := StoreRequest{
		Path:           "root.a.b",
		Content:        "hello",
		Metadata:       map[string]interface{}{"k": float64(1)}, // JSON numbers decode as float64
		Tags:           []string{"t1"},
		EmbeddingModel: "unspecified",
		EmbeddingSpace: "default",
		SourceAgentID:  "agent-1",
		EncryptContent: true,
		ExpiresAt:      &expires,
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out StoreRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Path != in.Path || out.Content != in.Content || out.EmbeddingModel != in.EmbeddingModel {
		t.Fatalf("decode mismatch: %+v", out)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "t1" {
		t.Fatalf("tags: %+v", out.Tags)
	}
	if out.ExpiresAt == nil || !out.ExpiresAt.Equal(expires) {
		t.Fatalf("expires: %v", out.ExpiresAt)
	}
}

func TestRetrieveRequestJSONRoundTrip(t *testing.T) {
	in := RetrieveRequest{
		PathPrefix: "root.x",
		Query:      "q",
		Limit:      25,
		Tags:       []string{"a"},
		TagsMatch:  "all",
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out RetrieveRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.PathPrefix != in.PathPrefix || out.Limit != in.Limit || out.TagsMatch != in.TagsMatch {
		t.Fatalf("%+v", out)
	}
}

func TestRollbackRequestJSONRoundTrip(t *testing.T) {
	v := 3
	in := RollbackRequest{Path: "p", Version: &v}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out RollbackRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Path != "p" || out.Version == nil || *out.Version != 3 {
		t.Fatalf("%+v", out)
	}
}

func TestCompactMemoryRequestJSONRoundTrip(t *testing.T) {
	in := CompactMemoryRequest{Path: "root.z", KeepSuperseded: 10}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out CompactMemoryRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Path != in.Path || out.KeepSuperseded != 10 {
		t.Fatalf("%+v", out)
	}
}

func TestTenantCreateRequestJSONRoundTrip(t *testing.T) {
	in := TenantCreateRequest{
		Slug: "acme", Name: "ACME", Settings: map[string]interface{}{"x": true},
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out TenantCreateRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Slug != "acme" || out.Name != "ACME" {
		t.Fatalf("%+v", out)
	}
}

func TestMemoryImportRequestJSONRoundTrip(t *testing.T) {
	in := MemoryImportRequest{
		Mode: "skip",
		Entries: []StoreRequest{
			{Path: "a.b", Content: "c", EmbeddingModel: "unspecified"},
		},
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out MemoryImportRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Mode != "skip" || len(out.Entries) != 1 {
		t.Fatalf("%+v", out)
	}
}

func TestBatchStoreRequestJSONRoundTrip(t *testing.T) {
	in := BatchStoreRequest{
		Items: []StoreRequest{
			{Path: "p1", Content: "a", EmbeddingModel: "unspecified"},
		},
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var out BatchStoreRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Path != "p1" {
		t.Fatalf("%+v", out)
	}
}
