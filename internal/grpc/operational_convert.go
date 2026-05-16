package grpcserver

import (
	"encoding/json"
	"fmt"
	"strings"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

func createLinkProtoToModel(req *pcmiv1.CreateLinkRequest) (model.CreateLinkRequest, error) {
	meta := map[string]interface{}{}
	if req.GetMetadataJson() != "" {
		_ = json.Unmarshal([]byte(req.GetMetadataJson()), &meta)
	}
	lt := strings.TrimSpace(req.GetLinkType())
	if lt == "" {
		lt = "related"
	}
	return model.CreateLinkRequest{
		FromPath: strings.TrimSpace(req.GetFromPath()),
		ToPath:   strings.TrimSpace(req.GetToPath()),
		LinkType: lt,
		Metadata: meta,
	}, nil
}

func memoryLinkToProto(l *model.MemoryLink) *pcmiv1.MemoryLinkMsg {
	if l == nil {
		return nil
	}
	return &pcmiv1.MemoryLinkMsg{
		Id: l.ID, FromPath: l.FromPath, ToPath: l.ToPath, LinkType: l.LinkType,
		MetadataJson: metadataToJSON(l.Metadata),
		CreatedAtRfc3339: formatTimeRFC3339(l.CreatedAt),
	}
}

func statsToProto(st *model.StatsResponse) *pcmiv1.StatsResponse {
	if st == nil {
		return &pcmiv1.StatsResponse{}
	}
	return &pcmiv1.StatsResponse{
		ActiveMemories: int32(st.ActiveMemories), SupersededMemories: int32(st.SupersededMemories),
		DistilledCount: int32(st.DistilledCount), LinksCount: int32(st.LinksCount),
		EventsCount: int32(st.EventsCount), ExpiringSoon: int32(st.ExpiringSoon),
	}
}

func ingestEventProtoToModel(req *pcmiv1.IngestEventRequest) (*model.IngestEventRequest, error) {
	payload := map[string]interface{}{}
	if req.GetPayloadJson() != "" {
		if err := json.Unmarshal([]byte(req.GetPayloadJson()), &payload); err != nil {
			return nil, err
		}
	}
	return &model.IngestEventRequest{
		EventType: req.GetEventType(), AgentID: req.GetAgentId(),
		CorrelationID: req.GetCorrelationId(), Payload: payload,
	}, nil
}

func ingestEventToProto(res *model.IngestEventResponse) *pcmiv1.IngestEventResponse {
	return &pcmiv1.IngestEventResponse{
		Id: res.ID, EventType: res.EventType, Status: res.Status,
		TimestampRfc3339: formatTimeRFC3339(res.Timestamp),
	}
}

func rollbackProtoToModel(req *pcmiv1.RollbackRequest) (*model.RollbackRequest, error) {
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	var ver *int
	if v := req.GetVersion(); v > 0 {
		n := int(v)
		ver = &n
	}
	asOf, err := parseAsOfRFC3339(req.GetAsOfRfc3339())
	if err != nil {
		return nil, err
	}
	return &model.RollbackRequest{Path: path, Version: ver, AsOf: asOf}, nil
}

func rollbackToProto(res *model.RollbackResponse) *pcmiv1.RollbackResponse {
	out := &pcmiv1.RollbackResponse{
		Id: res.ID, Status: res.Status, Version: int32(res.Version),
		RestoredFromVersion: int32(res.RestoredFromVersion),
	}
	if res.SupersededID != nil {
		sid := *res.SupersededID
		out.SupersededId = &sid
	}
	return out
}

func summarizeToProto(res *service.SummarizeResponse) *pcmiv1.SummarizeResponse {
	if res == nil {
		return &pcmiv1.SummarizeResponse{}
	}
	return &pcmiv1.SummarizeResponse{
		PathPrefix: res.PathPrefix, Summary: res.Summary, Method: res.Method, Total: int32(res.Total),
		SourceIds: res.SourceIDs,
	}
}
