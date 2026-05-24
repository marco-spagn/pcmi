package grpcserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/eventschema"
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

func (s *memoryServer) Refine(ctx context.Context, req *pcmiv1.RefineRequest) (*pcmiv1.RefineResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(req.GetPathPrefix())
	if prefix == "" {
		return nil, status.Error(codes.InvalidArgument, "path_prefix is required")
	}
	payload := map[string]any{
		"tenant_id": tenantID, "path_prefix": prefix,
		"requested_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := event.PublishEvent(event.EventMemoryRefineRequested, payload); err != nil {
		return nil, status.Error(codes.Unavailable, "failed to queue refine job")
	}
	return &pcmiv1.RefineResponse{Status: "queued", PathPrefix: prefix}, nil
}

func (s *memoryServer) CreateLink(ctx context.Context, req *pcmiv1.CreateLinkRequest) (*pcmiv1.CreateLinkResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	mr, err := createLinkProtoToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	link, err := s.linksRepo.Create(ctx, tenantID, mr)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &pcmiv1.CreateLinkResponse{Link: memoryLinkToProto(link)}, nil
}

func (s *memoryServer) ListLinks(ctx context.Context, req *pcmiv1.ListLinksRequest) (*pcmiv1.ListLinksResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	links, _, err := s.linksRepo.List(ctx, tenantID, req.GetFromPath(), req.GetToPath(), req.GetLinkType(), model.PageRequest{Limit: limit})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list links: %v", err)
	}
	out := &pcmiv1.ListLinksResponse{Total: int32(len(links))}
	for i := range links {
		out.Entries = append(out.Entries, memoryLinkToProto(&links[i]))
	}
	return out, nil
}

func (s *memoryServer) GetStats(ctx context.Context, req *pcmiv1.GetStatsRequest) (*pcmiv1.StatsResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	st, err := s.statsRepo.TenantStats(ctx, tenantID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "stats: %v", err)
	}
	return statsToProto(st), nil
}

func (s *memoryServer) IngestEvent(ctx context.Context, req *pcmiv1.IngestEventRequest) (*pcmiv1.IngestEventResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	ir, err := ingestEventProtoToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	res, err := s.eventSvc.Ingest(ctx, ir, tenantID)
	if err != nil {
		return nil, mapSvcValidationErr("ingest event", err)
	}
	return ingestEventToProto(res), nil
}

func (s *memoryServer) ListEventSchemas(context.Context, *pcmiv1.ListEventSchemasRequest) (*pcmiv1.ListEventSchemasResponse, error) {
	schemas := eventschema.List()
	out := &pcmiv1.ListEventSchemasResponse{Total: int32(len(schemas))}
	for _, sch := range schemas {
		fieldsJSON, _ := json.Marshal(sch.Fields)
		out.Schemas = append(out.Schemas, &pcmiv1.EventSchemaMsg{
			Type:        sch.EventType,
			Description: sch.Description,
			PayloadJson: string(fieldsJSON),
		})
	}
	return out, nil
}

func (s *memoryServer) StreamEvents(req *pcmiv1.StreamEventsRequest, stream pcmiv1.MemoryService_StreamEventsServer) error {
	ctx := stream.Context()
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return err
	}
	allowed := parseEventTypesGRPC(req.GetTypes())
	events := event.SubscribeEventsContext(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-events:
			if !ok {
				return nil
			}
			if !eventAllowedGRPC(evt, allowed, tenantID) {
				continue
			}
			payloadJSON, err := json.Marshal(evt.Payload)
			if err != nil {
				continue
			}
			if err := stream.Send(&pcmiv1.StreamEventMsg{
				Type: evt.Type, PayloadJson: string(payloadJSON),
			}); err != nil {
				return err
			}
		}
	}
}

func (s *memoryServer) RegisterWebhook(ctx context.Context, req *pcmiv1.RegisterWebhookRequest) (*pcmiv1.RegisterWebhookResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	url := strings.TrimSpace(req.GetUrl())
	if url == "" {
		return nil, status.Error(codes.InvalidArgument, "url is required")
	}
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, status.Errorf(codes.Internal, "tenant context: %v", err)
	}
	types := req.GetEventTypes()
	if types == nil {
		types = []string{}
	}
	var id string
	err = s.db.QueryRow(ctx, `
		INSERT INTO webhook_endpoints (tenant_id, url, event_types, secret)
		VALUES ($1::uuid, $2, $3, NULLIF($4, ''))
		RETURNING id::text`,
		tenantID, url, types, req.GetSecret(),
	).Scan(&id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "register webhook: %v", err)
	}
	return &pcmiv1.RegisterWebhookResponse{Id: id, Status: "registered"}, nil
}

func (s *memoryServer) ListWebhooks(ctx context.Context, req *pcmiv1.ListWebhooksRequest) (*pcmiv1.ListWebhooksResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, status.Errorf(codes.Internal, "tenant context: %v", err)
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, url, event_types, enabled, created_at
		FROM webhook_endpoints WHERE tenant_id = $1::uuid ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list webhooks: %v", err)
	}
	defer rows.Close()
	out := &pcmiv1.ListWebhooksResponse{}
	for rows.Next() {
		var id, url string
		var types []string
		var enabled bool
		var createdAt time.Time
		if err := rows.Scan(&id, &url, &types, &enabled, &createdAt); err != nil {
			continue
		}
		out.Entries = append(out.Entries, &pcmiv1.WebhookEndpointMsg{
			Id: id, Url: url, EventTypes: types, Enabled: enabled,
			CreatedAtRfc3339: formatTimeRFC3339(createdAt),
		})
	}
	out.Total = int32(len(out.Entries))
	return out, nil
}

func (s *memoryServer) ListWebhookDeadLetter(ctx context.Context, req *pcmiv1.ListWebhookDeadLetterRequest) (*pcmiv1.JSONResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, status.Errorf(codes.Internal, "tenant context: %v", err)
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `
		SELECT wd.id::text, wd.endpoint_id::text, wd.event_type, wd.payload,
		       wd.attempts, wd.last_error, wd.created_at
		FROM webhook_deliveries wd
		WHERE wd.tenant_id = $1::uuid AND wd.status = 'dead_letter'
		ORDER BY wd.created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "dead letter: %v", err)
	}
	defer rows.Close()
	entries := make([]map[string]any, 0)
	for rows.Next() {
		var id, endpointID, eventType, lastError string
		var payload any
		var attempts int
		var createdAt any
		if err := rows.Scan(&id, &endpointID, &eventType, &payload, &attempts, &lastError, &createdAt); err != nil {
			continue
		}
		entries = append(entries, map[string]any{
			"id": id, "endpoint_id": endpointID, "event_type": eventType,
			"payload": payload, "attempts": attempts, "last_error": lastError, "created_at": createdAt,
		})
	}
	return toJSONResponse(map[string]any{"entries": entries, "total": len(entries)})
}

func (s *memoryServer) MigrateEmbeddings(ctx context.Context, req *pcmiv1.MigrateEmbeddingsRequest) (*pcmiv1.MigrateEmbeddingsResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(req.GetPathPrefix())
	if prefix == "" {
		return nil, status.Error(codes.InvalidArgument, "path_prefix is required")
	}
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, status.Errorf(codes.Internal, "tenant context: %v", err)
	}
	var space *string
	if sp := strings.TrimSpace(req.GetEmbeddingSpace()); sp != "" {
		space = &sp
	}
	var n int
	err = s.db.QueryRow(ctx,
		`SELECT mark_embeddings_for_migration($1::uuid, $2, $3, $4)`,
		tenantID, prefix, req.GetTargetModel(), space,
	).Scan(&n)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "migrate embeddings: %v", err)
	}
	return &pcmiv1.MigrateEmbeddingsResponse{
		Status: "queued", MarkedCount: int32(n), PathPrefix: prefix,
		TargetModel: req.GetTargetModel(), EmbeddingSpace: req.GetEmbeddingSpace(),
	}, nil
}

func (s *memoryServer) Rollback(ctx context.Context, req *pcmiv1.RollbackRequest) (*pcmiv1.RollbackResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	rr, err := rollbackProtoToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	res, err := s.svc.Rollback(ctx, rr, tenantID)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not found") || strings.Contains(msg, "no historical version") {
			return nil, status.Error(codes.NotFound, msg)
		}
		return nil, status.Errorf(codes.Internal, "rollback: %v", err)
	}
	return rollbackToProto(res), nil
}

func (s *memoryServer) Summarize(ctx context.Context, req *pcmiv1.SummarizeRequest) (*pcmiv1.SummarizeResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	sr := &service.SummarizeRequest{
		PathPrefix: req.GetPathPrefix(),
		Limit:      int(req.GetLimit()),
		Style:      req.GetStyle(),
	}
	res, err := s.summarize.Summarize(ctx, sr, tenantID)
	if err != nil {
		return nil, mapSvcValidationErr("summarize", err)
	}
	return summarizeToProto(res), nil
}

func (s *memoryServer) GetHistory(ctx context.Context, req *pcmiv1.GetHistoryRequest) (*pcmiv1.GetHistoryResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	entries, _, err := s.memRepo.ListPathHistory(ctx, tenantID, path, model.PageRequest{Limit: limit})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "history: %v", err)
	}
	out := &pcmiv1.GetHistoryResponse{Path: path, Total: int32(len(entries))}
	for i := range entries {
		out.Entries = append(out.Entries, memoryEntryToProtoRetrieve(&entries[i]))
	}
	return out, nil
}

func (s *memoryServer) GetMemoryLineage(ctx context.Context, req *pcmiv1.GetMemoryLineageRequest) (*pcmiv1.JSONResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	res, err := s.lineageRepo.MemoryLineage(ctx, tenantID, path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "memory lineage: %v", err)
	}
	return toJSONResponse(res)
}

func (s *memoryServer) GetDistilledLineage(ctx context.Context, req *pcmiv1.GetDistilledLineageRequest) (*pcmiv1.JSONResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if req.GetDistilledId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "distilled_id is required")
	}
	res, err := s.lineageRepo.DistilledLineage(ctx, tenantID, req.GetDistilledId())
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "distilled lineage: %v", err)
	}
	return toJSONResponse(res)
}

func (s *memoryServer) ListDistilled(ctx context.Context, req *pcmiv1.ListDistilledRequest) (*pcmiv1.JSONResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(req.GetPathPrefix())
	if prefix == "" {
		return nil, status.Error(codes.InvalidArgument, "path_prefix is required")
	}
	limit := int(req.GetLimit())
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, status.Errorf(codes.Internal, "tenant context: %v", err)
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, path::text, summary, insights, confidence_score, distilled_at, source_entry_ids, version
		FROM distilled_knowledge
		WHERE tenant_id = $1::uuid AND path <@ $2::ltree
		ORDER BY distilled_at DESC LIMIT $3`, tenantID, prefix, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list distilled: %v", err)
	}
	defer rows.Close()
	results := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id          int64
			path        string
			summary     string
			insightsRaw []byte
			confidence  sql.NullFloat64
			distilledAt time.Time
			sourceIDs   []int64
			version     int
		)
		if err := rows.Scan(&id, &path, &summary, &insightsRaw, &confidence, &distilledAt, &sourceIDs, &version); err != nil {
			continue
		}
		var insights any = json.RawMessage(insightsRaw)
		if len(insightsRaw) > 0 {
			var arr []any
			if json.Unmarshal(insightsRaw, &arr) == nil {
				insights = arr
			}
		}
		row := map[string]any{
			"id": id, "path": path, "summary": summary, "insights": insights,
			"distilled_at": distilledAt.Format(time.RFC3339), "source_entry_ids": sourceIDs, "version": version,
		}
		if confidence.Valid {
			row["confidence_score"] = confidence.Float64
		}
		results = append(results, row)
	}
	return toJSONResponse(map[string]any{"entries": results, "total": len(results), "tenant": tenantID})
}

func (s *memoryServer) ListAudit(ctx context.Context, req *pcmiv1.ListAuditRequest) (*pcmiv1.JSONResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	var since *time.Time
	if s := strings.TrimSpace(req.GetSinceRfc3339()); s != "" {
		t, perr := time.Parse(time.RFC3339, s)
		if perr != nil {
			t, perr = time.Parse(time.RFC3339Nano, s)
			if perr != nil {
				return nil, status.Error(codes.InvalidArgument, "since_rfc3339: invalid timestamp")
			}
		}
		since = &t
	}
	entries, pageResp, err := s.auditRepo.List(ctx, tenantID, model.PageRequest{Limit: limit}, since)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "audit: %v", err)
	}
	total, err := s.auditRepo.Count(ctx, tenantID, since)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "audit count: %v", err)
	}
	return toJSONResponse(map[string]any{
		"entries": entries, "total": total, "limit": limit, "offset": 0,
		"next_cursor": pageResp.NextCursor, "has_more": pageResp.HasMore,
	})
}

func (s *memoryServer) ExportMemories(ctx context.Context, req *pcmiv1.ExportMemoriesRequest) (*pcmiv1.ExportMemoriesResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	er := &model.MemoryExportRequest{
		PathPrefix: req.GetPathPrefix(),
		Limit:      int(req.GetLimit()),
		IncludeEmb: req.GetIncludeEmbeddings(),
	}
	res, err := s.svc.Export(ctx, tenantID, er)
	if err != nil {
		return nil, mapSvcValidationErr("export", err)
	}
	out := &pcmiv1.ExportMemoriesResponse{
		TenantId: tenantID, Exported: int32(res.Exported),
		ExportedAtRfc3339: formatTimeRFC3339(res.ExportedAt),
	}
	for i := range res.Entries {
		out.Entries = append(out.Entries, memoryEntryToProtoRetrieve(&res.Entries[i]))
	}
	return out, nil
}

func (s *memoryServer) ImportMemories(ctx context.Context, req *pcmiv1.ImportMemoriesRequest) (*pcmiv1.ImportMemoriesResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	items, err := batchStoreProtoToModel(req.GetItems())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	ir := &model.MemoryImportRequest{Entries: items, Mode: req.GetMode()}
	if ir.Mode == "" {
		ir.Mode = "skip"
	}
	res, err := s.svc.Import(ctx, tenantID, ir)
	if err != nil {
		return nil, mapSvcValidationErr("import", err)
	}
	out := &pcmiv1.ImportMemoriesResponse{Imported: int32(res.Imported), Skipped: int32(res.Skipped)}
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
	return out, nil
}

func parseEventTypesGRPC(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

func eventAllowedGRPC(evt event.Event, allowed map[string]struct{}, tenantID string) bool {
	if allowed != nil {
		if _, ok := allowed[evt.Type]; !ok {
			return false
		}
	}
	if tenantID == "" {
		return true
	}
	if evtTenant, ok := evt.Payload["tenant_id"].(string); ok && evtTenant != "" {
		return evtTenant == tenantID
	}
	return true
}
