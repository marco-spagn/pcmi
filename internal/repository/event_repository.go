package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EventRepository struct {
	db *pgxpool.Pool
}

func NewEventRepository(db *pgxpool.Pool) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Insert(
	ctx context.Context,
	tenantID, eventType string,
	payload map[string]interface{},
	agentID *string,
) (id string, ts time.Time, err error) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return "", time.Time{}, fmt.Errorf("event_type is required")
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["tenant_id"] = tenantID

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal payload: %w", err)
	}

	q := `
		INSERT INTO events (tenant_id, event_type, payload, agent_id)
		VALUES ($1::uuid, $2, $3::jsonb, $4::uuid)
		RETURNING id::text, timestamp`
	err = r.db.QueryRow(ctx, q, tenantID, eventType, raw, agentID).Scan(&id, &ts)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert event: %w", err)
	}
	return id, ts, nil
}
