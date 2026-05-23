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
	return parseRFC3339Time(s, "as_of_rfc3339")
}

func parseExpiresAtRFC3339(s string) (*time.Time, error) {
	return parseRFC3339Time(s, "expires_at_rfc3339")
}

func parseRFC3339Time(s, field string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
	}
	return &t, nil
}

func storeFieldsToModel(path, content, metadataJSON string, tags []string, embedding []float32, embeddingModel, embeddingSpace, sourceAgentID string, encryptContent bool, expiresAtRFC string, importance *float64) (model.StoreRequest, error) {
	expiresAt, err := parseExpiresAtRFC3339(expiresAtRFC)
	if err != nil {
		return model.StoreRequest{}, err
	}
	meta := map[string]interface{}{}
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &meta)
	}
	var tagsCopy []string
	if len(tags) > 0 {
		tagsCopy = append(tagsCopy, tags...)
	}
	var embCopy []float32
	if len(embedding) > 0 {
		embCopy = append(embCopy, embedding...)
	}
	out := model.StoreRequest{
		Path:           path,
		Content:        content,
		Metadata:       meta,
		Tags:           tagsCopy,
		Embedding:      embCopy,
		EmbeddingModel: strings.TrimSpace(embeddingModel),
		EmbeddingSpace: strings.TrimSpace(embeddingSpace),
		SourceAgentID:  strings.TrimSpace(sourceAgentID),
		EncryptContent: encryptContent,
		ExpiresAt:      expiresAt,
	}
	if importance != nil {
		out.Importance = importance
	}
	return out, nil
}

func storeProtoToModel(req *pcmiv1.StoreRequest) (model.StoreRequest, error) {
	if req == nil {
		return model.StoreRequest{}, nil
	}
	var imp *float64
	if req != nil && req.GetImportance() != 0 {
		v := req.GetImportance()
		imp = &v
	}
	return storeFieldsToModel(
		req.GetPath(), req.GetContent(), req.GetMetadataJson(),
		req.GetTags(), req.GetEmbedding(), req.GetEmbeddingModel(), req.GetEmbeddingSpace(),
		req.GetSourceAgentId(), req.GetEncryptContent(), req.GetExpiresAtRfc3339(),
		imp,
	)
}

func storeItemProtoToModel(it *pcmiv1.BatchStoreItem) (model.StoreRequest, error) {
	if it == nil {
		return model.StoreRequest{}, nil
	}
	return storeFieldsToModel(
		it.GetPath(), it.GetContent(), it.GetMetadataJson(),
		it.GetTags(), it.GetEmbedding(), it.GetEmbeddingModel(), it.GetEmbeddingSpace(),
		it.GetSourceAgentId(), it.GetEncryptContent(), it.GetExpiresAtRfc3339(),
		nil,
	)
}

func retrieveFieldsToModel(pathPrefix, query string, limit int32, tags []string, tagsMatch, asOfRFC, sourceAgentID, embeddingSpace string, decayEnabled *bool) (model.RetrieveRequest, error) {
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
		DecayEnabled:   decayEnabled,
	}, nil
}

func retrieveProtoToModel(req *pcmiv1.RetrieveRequest) (model.RetrieveRequest, error) {
	if req == nil {
		return model.RetrieveRequest{Limit: 10}, nil
	}
	var decay *bool
	if req != nil && req.GetDecayDisabled() {
		v := false
		decay = &v
	}
	return retrieveFieldsToModel(
		req.GetPathPrefix(), req.GetQuery(), req.GetLimit(),
		req.GetTags(), req.GetTagsMatch(), req.GetAsOfRfc3339(),
		req.GetSourceAgentId(), req.GetEmbeddingSpace(),
		decay,
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
			nil,
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

func formatTimeRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func metadataToJSON(meta any) string {
	if meta == nil {
		return "{}"
	}
	b, err := json.Marshal(meta)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}

func memoryEntryToProtoRetrieve(e *model.MemoryEntry) *pcmiv1.RetrieveEntry {
	if e == nil {
		return nil
	}
	out := &pcmiv1.RetrieveEntry{
		Id:               e.ID,
		TenantId:         e.TenantID,
		Path:             e.Path,
		Content:          e.Content,
		Version:          int32(e.Version),
		RelevanceScore:   e.RelevanceScore,
		MetadataJson:     metadataToJSON(e.Metadata),
		EmbeddingModel:   e.EmbeddingModel,
		EmbeddingSpace:   e.EmbeddingSpace,
		ValidFromRfc3339: formatTimeRFC3339(e.ValidFrom),
		CreatedAtRfc3339: formatTimeRFC3339(e.CreatedAt),
		ContentEncrypted: e.ContentEncrypted,
	}
	if len(e.Tags) > 0 {
		out.Tags = append([]string(nil), e.Tags...)
	}
	if e.ValidTo != nil {
		out.ValidToRfc3339 = formatTimeRFC3339(*e.ValidTo)
	}
	if e.SourceAgentID != nil {
		out.SourceAgentId = *e.SourceAgentID
	}
	if e.SourceEventID != nil {
		out.SourceEventId = *e.SourceEventID
	}
	if len(e.Embedding) > 0 {
		out.Embedding = append([]float32(nil), e.Embedding...)
	}
	out.Importance = e.Importance
	if e.AccessCount > 0 {
		out.AccessCount = int32(e.AccessCount)
	}
	if e.LastAccessedAt != nil {
		out.LastAccessedAtRfc3339 = formatTimeRFC3339(*e.LastAccessedAt)
	}
	return out
}

func batchStoreProtoToModel(items []*pcmiv1.BatchStoreItem) ([]model.StoreRequest, error) {
	out := make([]model.StoreRequest, 0, len(items))
	for _, it := range items {
		m, err := storeItemProtoToModel(it)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func storeResultToProto(id int64, version int, supersededID *int64) *pcmiv1.StoreResponse {
	out := &pcmiv1.StoreResponse{
		Id: id, Status: "stored", Version: int32(version),
	}
	if supersededID != nil {
		sid := *supersededID
		out.SupersededId = &sid
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

func getMemoryProtoToParams(req *pcmiv1.GetMemoryRequest) (path string, version *int, asOf *time.Time, err error) {
	if req == nil {
		return "", nil, nil, fmt.Errorf("path is required")
	}
	path = strings.TrimSpace(req.GetPath())
	if path == "" {
		return "", nil, nil, fmt.Errorf("path is required")
	}
	if v := req.GetVersion(); v > 0 {
		n := int(v)
		version = &n
	}
	asOf, err = parseAsOfRFC3339(req.GetAsOfRfc3339())
	if err != nil {
		return "", nil, nil, err
	}
	return path, version, asOf, nil
}

func compactProtoToModel(req *pcmiv1.CompactRequest) (model.CompactMemoryRequest, error) {
	if req == nil {
		return model.CompactMemoryRequest{}, fmt.Errorf("path is required")
	}
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		return model.CompactMemoryRequest{}, fmt.Errorf("path is required")
	}
	keep := int(req.GetKeepSuperseded())
	if keep <= 0 {
		keep = 20
	}
	return model.CompactMemoryRequest{Path: path, KeepSuperseded: keep}, nil
}

func compactModelToProto(res *model.CompactMemoryResponse) *pcmiv1.CompactResponse {
	return &pcmiv1.CompactResponse{
		Path:           res.Path,
		DeletedCount:   int32(res.DeletedCount),
		KeepSuperseded: int32(res.KeepSuperseded),
	}
}
