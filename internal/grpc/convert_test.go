package grpcserver

import (
	"testing"
	"time"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
)

func TestDefaultRetrieveLimit(t *testing.T) {
	if defaultRetrieveLimit(0) != 10 || defaultRetrieveLimit(-1) != 10 {
		t.Fatal("expected 10 for non-positive")
	}
	if defaultRetrieveLimit(5) != 5 {
		t.Fatal("expected 5")
	}
}

func TestBatchQueriesProtoToModel(t *testing.T) {
	qs := []*pcmiv1.BatchRetrieveQuery{
		{PathPrefix: "a", Query: "q1", Limit: 0, Tags: []string{"x"}, TagsMatch: "all"},
		nil,
		{PathPrefix: "b", Query: "", Limit: 3, SourceAgentId: "agent-1", EmbeddingSpace: "s1"},
	}
	got, err := batchQueriesProtoToModel(qs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].PathPrefix != "a" || got[0].Query != "q1" || got[0].Limit != 10 {
		t.Fatalf("first: %#v", got[0])
	}
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "x" || got[0].TagsMatch != "all" {
		t.Fatalf("tags: %#v", got[0])
	}
	if got[1].Limit != 10 {
		t.Fatalf("nil query default limit")
	}
	if got[2].PathPrefix != "b" || got[2].Limit != 3 {
		t.Fatalf("third: %#v", got[2])
	}
	if got[2].SourceAgentID != "agent-1" || got[2].EmbeddingSpace != "s1" {
		t.Fatalf("scope: %#v", got[2])
	}
}

func TestBatchQueriesProtoToModel_invalidAsOf(t *testing.T) {
	_, err := batchQueriesProtoToModel([]*pcmiv1.BatchRetrieveQuery{
		{AsOfRfc3339: "not-a-date"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRetrieveProtoToModel_asOf(t *testing.T) {
	req := &pcmiv1.RetrieveRequest{
		PathPrefix: "p", Limit: 5,
		AsOfRfc3339: "2020-01-02T15:04:05Z",
	}
	m, err := retrieveProtoToModel(req)
	if err != nil {
		t.Fatal(err)
	}
	if m.AsOf == nil || m.AsOf.UTC().Format(time.RFC3339) != "2020-01-02T15:04:05Z" {
		t.Fatalf("%v", m.AsOf)
	}
}

func TestBatchRetrieveModelToProto(t *testing.T) {
	res := &model.BatchRetrieveResponse{
		Total: 1,
		Results: []model.RetrieveResponse{
			{
				Total: 2,
				Entries: []model.MemoryEntry{
					{ID: 1, Path: "p", Content: "c", Version: 1, RelevanceScore: 0.5},
					{ID: 2, Path: "p2", Content: "c2", Version: 1},
				},
			},
		},
	}
	pb := batchRetrieveModelToProto(res)
	if pb.GetTotal() != 1 || len(pb.GetResults()) != 1 {
		t.Fatal(pb)
	}
	r0 := pb.GetResults()[0]
	if r0.GetTotal() != 2 || len(r0.GetEntries()) != 2 {
		t.Fatal(r0)
	}
	if r0.GetEntries()[0].GetId() != 1 || r0.GetEntries()[0].GetRelevanceScore() != 0.5 {
		t.Fatal(r0.GetEntries()[0])
	}
}

func TestBatchStoreProtoToModel(t *testing.T) {
	items := []*pcmiv1.BatchStoreItem{
		{
			Path: "a", Content: "c", MetadataJson: `{"k":1}`,
			Tags: []string{"ci"}, EmbeddingModel: "text-embedding-3-small",
			SourceAgentId: "agent-1", EncryptContent: true,
			ExpiresAtRfc3339: "2030-01-02T15:04:05Z",
		},
		nil,
	}
	got, err := batchStoreProtoToModel(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatal(len(got))
	}
	if got[0].Path != "a" || got[0].Content != "c" {
		t.Fatal(got[0])
	}
	if v, ok := got[0].Metadata["k"].(float64); !ok || v != 1 {
		t.Fatalf("metadata %#v", got[0].Metadata)
	}
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "ci" {
		t.Fatalf("tags %#v", got[0].Tags)
	}
	if got[0].EmbeddingModel != "text-embedding-3-small" || got[0].SourceAgentID != "agent-1" || !got[0].EncryptContent {
		t.Fatalf("fields %#v", got[0])
	}
	if got[0].ExpiresAt == nil {
		t.Fatal("expires_at")
	}
}

func TestStoreProtoToModel_invalidExpires(t *testing.T) {
	_, err := storeProtoToModel(&pcmiv1.StoreRequest{ExpiresAtRfc3339: "bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStoreProtoToModel_full(t *testing.T) {
	m, err := storeProtoToModel(&pcmiv1.StoreRequest{
		Path: "p", Content: "c", MetadataJson: `{"k":1}`,
		Tags: []string{"a", "b"}, EmbeddingModel: "m1", EmbeddingSpace: "space1",
		SourceAgentId: "agent", EncryptContent: true,
		ExpiresAtRfc3339: "2030-06-01T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Path != "p" || len(m.Tags) != 2 || !m.EncryptContent || m.ExpiresAt == nil {
		t.Fatalf("%#v", m)
	}
}

func TestStoreItemProtoToModel_nil(t *testing.T) {
	m, err := storeItemProtoToModel(nil)
	if err != nil || m.Path != "" {
		t.Fatalf("%#v err=%v", m, err)
	}
}

func TestParseRFC3339Time_empty(t *testing.T) {
	tm, err := parseRFC3339Time("", "field")
	if err != nil || tm != nil {
		t.Fatalf("tm=%v err=%v", tm, err)
	}
}

func TestBatchStoreProtoToModel_invalidExpires(t *testing.T) {
	_, err := batchStoreProtoToModel([]*pcmiv1.BatchStoreItem{{ExpiresAtRfc3339: "bad"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStoreResultToProto_superseded(t *testing.T) {
	sid := int64(42)
	pb := storeResultToProto(1, 2, &sid)
	if pb.GetId() != 1 || pb.GetVersion() != 2 || pb.GetSupersededId() != 42 {
		t.Fatal(pb)
	}
}

func TestBatchStoreModelToProto(t *testing.T) {
	sid := int64(99)
	res := &model.BatchStoreResult{
		Total: 1,
		Results: []model.BatchStoreItemResult{
			{Index: 0, ID: 1, Status: "stored", Version: 2, SupersededID: &sid},
			{Index: 1, Status: "error", Error: "boom"},
		},
	}
	pb := batchStoreModelToProto(res)
	if pb.GetTotal() != 1 || len(pb.GetResults()) != 2 {
		t.Fatal(pb)
	}
	if pb.GetResults()[0].GetSupersededId() != 99 {
		t.Fatal(pb.GetResults()[0])
	}
	if pb.GetResults()[1].GetError() != "boom" {
		t.Fatal(pb.GetResults()[1])
	}
}
