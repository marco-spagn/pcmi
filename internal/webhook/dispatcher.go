package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Dispatcher struct {
	db     *pgxpool.Pool
	client *http.Client
}

func NewDispatcher(db *pgxpool.Pool) *Dispatcher {
	return &Dispatcher{
		db: db,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

type endpoint struct {
	ID          string
	URL         string
	EventTypes  []string
	Secret      string
}

// NotifyMatching delivers event payloads to tenant webhooks (async, best-effort).
func (d *Dispatcher) NotifyMatching(tenantID, eventType string, payload map[string]any) {
	if d == nil || d.db == nil || tenantID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		eps, err := d.listEndpoints(ctx, tenantID, eventType)
		if err != nil {
			log.Printf("webhook list: %v", err)
			return
		}
		body, err := json.Marshal(map[string]any{
			"event_type": eventType,
			"tenant_id":  tenantID,
			"payload":    payload,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return
		}
		var wg sync.WaitGroup
		for _, ep := range eps {
			wg.Add(1)
			go func(e endpoint) {
				defer wg.Done()
				d.deliver(ctx, e, body)
			}(ep)
		}
		wg.Wait()
	}()
}

func (d *Dispatcher) listEndpoints(ctx context.Context, tenantID, eventType string) ([]endpoint, error) {
	if _, err := d.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return nil, err
	}
	rows, err := d.db.Query(ctx, `
		SELECT id::text, url, event_types, COALESCE(secret, '')
		FROM webhook_endpoints
		WHERE tenant_id = $1::uuid AND enabled = TRUE
		  AND (
		    cardinality(event_types) = 0
		    OR $2 = ANY(event_types)
		  )`, tenantID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []endpoint
	for rows.Next() {
		var e endpoint
		if err := rows.Scan(&e.ID, &e.URL, &e.EventTypes, &e.Secret); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *Dispatcher) deliver(ctx context.Context, ep endpoint, body []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook %s build req: %v", ep.ID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PCMI-Event-Delivery", "1")
	if ep.Secret != "" {
		mac := hmac.New(sha256.New, []byte(ep.Secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-PCMI-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("webhook %s POST %s: %v", ep.ID, ep.URL, err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("webhook %s POST %s: status %d", ep.ID, ep.URL, resp.StatusCode)
	}
}
