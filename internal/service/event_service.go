package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/eventschema"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type EventService struct {
	repo *repository.EventRepository
}

func NewEventService(repo *repository.EventRepository) *EventService {
	return &EventService{repo: repo}
}

func (s *EventService) Ingest(ctx context.Context, req *model.IngestEventRequest, tenantID string) (*model.IngestEventResponse, error) {
	eventType := strings.TrimSpace(req.EventType)
	if eventType == "" {
		return nil, fmt.Errorf("event_type is required")
	}

	payload := req.Payload
	if err := eventschema.Validate(eventType, payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if req.CorrelationID != "" {
		payload["correlation_id"] = req.CorrelationID
	}

	var agentID *string
	if a := strings.TrimSpace(req.AgentID); a != "" {
		agentID = &a
	}

	id, ts, err := s.repo.Insert(ctx, tenantID, eventType, payload, agentID)
	if err != nil {
		return nil, err
	}

	redisPayload := map[string]any{
		"id":        id,
		"tenant_id": tenantID,
		"timestamp": ts.UTC().Format(time.RFC3339),
	}
	for k, v := range payload {
		redisPayload[k] = v
	}
	if agentID != nil {
		redisPayload["agent_id"] = *agentID
	}
	_ = event.PublishEvent(eventType, redisPayload)

	return &model.IngestEventResponse{
		ID:        id,
		EventType: eventType,
		Timestamp: ts,
		Status:    "ingested",
	}, nil
}
