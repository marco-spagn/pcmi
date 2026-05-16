package grpcserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

func parseAsOfRFC3339(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("as_of_rfc3339: %w", err)
		}
	}
	return &t, nil
}

func retrieveFieldsToModel(pathPrefix, query string, limit int32, tags []string, tagsMatch, asOfRFC, sourceAgentID, embeddingSpace string) (model.RetrieveRequest, error) {
	asOf, err := parseAsOfRFC3339(asOfRFC)
	if err != nil {
		return model.RetrieveRequest{}, err
	}
	var tagsCopy []string
	if len(tags) > 0 {
		tagsCopy = append(tagsCopy, tags...)
	}
	return model.RetrieveRequest{
		PathPrefix:     pathPrefix,
		Query:          query,
		Limit:          defaultRetrieveLimit(limit),
		Tags:           tagsCopy,
		TagsMatch:      tagsMatch,
		AsOf:           asOf,
		SourceAgentID:  strings.TrimSpace(sourceAgentID),
		EmbeddingSpace: strings.TrimSpace(embeddingSpace),
	}, nil
}

func retrieveProtoToModel(req *pcmiv1.RetrieveRequest) (model.RetrieveRequest, error) {
	if req == nil {
		return model.RetrieveRequest{Limit: 10}, nil
	}
	return retrieveFieldsToModel(
		req.GetPathPrefix(), req.GetQuery(), req.GetLimit(),
		req.GetTags(), req.GetTagsMatch(), req.GetAsOfRfc3339(),
		req.GetSourceAgentId(), req.GetEmbeddingSpace(),
	)
}

func batchQueriesProtoToModel(queries []*pcmiv1.BatchRetrieveQuery) ([]model.RetrieveRequest, error) {
	out := make([]model.RetrieveRequest, 0, len(queries))
	for _, q := range queries {
		if q == nil {
			out = append(out, model.RetrieveRequest{Limit: 10})
			continue
		}
		m, err := retrieveFieldsToModel(
			q.GetPathPrefix(), q.GetQuery(), q.GetLimit(),
			q.GetTags(), q.GetTagsMatch(), q.GetAsOfRfc3339(),
			q.GetSourceAgentId(), q.GetEmbeddingSpace(),
		)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
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

func batchStoreProtoToModel(items []*pcmiv1.BatchStoreItem) []model.StoreRequest {
	out := make([]model.StoreRequest, 0, len(items))
	for _, it := range items {
		if it == nil {
			out = append(out, model.StoreRequest{})
			continue
		}
		meta := map[string]interface{}{}
		if it.GetMetadataJson() != "" {
			_ = json.Unmarshal([]byte(it.GetMetadataJson()), &meta)
		}
		out = append(out, model.StoreRequest{
			Path: it.GetPath(), Content: it.GetContent(), Metadata: meta,
		})
	}
	return out
}

func batchStoreModelToProto(res *model.BatchStoreResult) *pcmiv1.BatchStoreResponse {
	out := &pcmiv1.BatchStoreResponse{Total: int32(res.Total)}
	for _, r := range res.Results {
		br := &pcmiv1.BatchStoreItemResult{
			Index: int32(r.Index), Id: r.ID, Status: r.Status,
			Version: int32(r.Version), Error: r.Error,
		}
		if r.SupersededID != nil {
			sid := *r.SupersededID
			br.SupersededId = &sid
		}
		out.Results = append(out.Results, br)
	}
	return out
}
