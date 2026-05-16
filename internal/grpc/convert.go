package grpcserver

import (
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
)

func defaultRetrieveLimit(v int32) int {
	n := int(v)
	if n <= 0 {
		return 10
	}
	return n
}

func batchQueriesProtoToModel(queries []*pcmiv1.BatchRetrieveQuery) []model.RetrieveRequest {
	out := make([]model.RetrieveRequest, 0, len(queries))
	for _, q := range queries {
		if q == nil {
			out = append(out, model.RetrieveRequest{Limit: 10})
			continue
		}
		out = append(out, model.RetrieveRequest{
			PathPrefix: q.GetPathPrefix(),
			Query:      q.GetQuery(),
			Limit:      defaultRetrieveLimit(q.GetLimit()),
		})
	}
	return out
}

func batchRetrieveModelToProto(res *model.BatchRetrieveResponse) *pcmiv1.BatchRetrieveResponse {
	out := &pcmiv1.BatchRetrieveResponse{Total: int32(res.Total)}
	for _, r := range res.Results {
		br := &pcmiv1.BatchRetrieveResult{Total: int32(r.Total)}
		for i := range r.Entries {
			br.Entries = append(br.Entries, memoryEntryToProtoRetrieve(&r.Entries[i]))
		}
		out.Results = append(out.Results, br)
	}
	return out
}

func memoryEntryToProtoRetrieve(e *model.MemoryEntry) *pcmiv1.RetrieveEntry {
	if e == nil {
		return nil
	}
	return &pcmiv1.RetrieveEntry{
		Id:             e.ID,
		Path:           e.Path,
		Content:        e.Content,
		Version:        int32(e.Version),
		RelevanceScore: e.RelevanceScore,
	}
}
