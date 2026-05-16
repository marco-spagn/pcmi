package grpcserver

import (
	"testing"

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
		{PathPrefix: "a", Query: "q1", Limit: 0},
		nil,
		{PathPrefix: "b", Query: "", Limit: 3},
	}
	got := batchQueriesProtoToModel(qs)
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].PathPrefix != "a" || got[0].Query != "q1" || got[0].Limit != 10 {
		t.Fatalf("first: %#v", got[0])
	}
	if got[1].Limit != 10 {
		t.Fatalf("nil query default limit")
	}
	if got[2].PathPrefix != "b" || got[2].Limit != 3 {
		t.Fatalf("third: %#v", got[2])
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
