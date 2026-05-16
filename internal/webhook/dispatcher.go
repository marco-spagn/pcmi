package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultMaxAttempts = 5

type Dispatcher struct {
	db           *pgxpool.Pool
	client       *http.Client
	maxAttempts  int
	retryBase    time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

func NewDispatcher(db *pgxpool.Pool) *Dispatcher {
	max := defaultMaxAttempts
	if v := os.Getenv("WEBHOOK_MAX_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			max = n
		}
	}
	d := &Dispatcher{
		db: db,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
		maxAttempts: max,
		retryBase:   2 * time.Second,
		stopCh:      make(chan struct{}),
	}
	d.wg.Add(1)
	go d.retryLoop()
	return d
}

func (d *Dispatcher) Close() {
	close(d.stopCh)
	d.wg.Wait()
}

type endpoint struct {
	ID         string
	URL        string
	EventTypes []string
	Secret     string
}

type pendingDelivery struct {
	ID         string
	EndpointID string
	URL        string
	Secret     string
	EventType  string
	Body       []byte
	Attempts   int
}

// NotifyMatching enqueues webhook deliveries for matching tenant endpoints.
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
		for _, ep := range eps {
			if err := d.enqueue(ctx, tenantID, ep, eventType, payload, body); err != nil {
				log.Printf("webhook enqueue %s: %v", ep.ID, err)
			}
		}
	}()
}

func (d *Dispatcher) enqueue(ctx context.Context, tenantID string, ep endpoint, eventType string, payload map[string]any, body []byte) error {
	if _, err := d.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(ctx, `
		INSERT INTO webhook_deliveries (tenant_id, endpoint_id, event_type, payload, max_attempts, next_retry_at)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, $5, NOW())`,
		tenantID, ep.ID, eventType, payloadJSON, d.maxAttempts,
	)
	if err != nil {
		return err
	}
	go d.processPending()
	return nil
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

func (d *Dispatcher) retryLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.processPending()
		}
	}
}

func (d *Dispatcher) processPending() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := d.db.Query(ctx, `
		SELECT wd.id::text, wd.endpoint_id::text, we.url, COALESCE(we.secret, ''),
		       wd.event_type, wd.attempts, wd.payload
		FROM webhook_deliveries wd
		JOIN webhook_endpoints we ON we.id = wd.endpoint_id
		WHERE wd.status = 'pending'
		  AND wd.next_retry_at <= NOW()
		  AND wd.attempts < wd.max_attempts
		ORDER BY wd.next_retry_at
		LIMIT 20
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		log.Printf("webhook pending query: %v", err)
		return
	}
	defer rows.Close()

	var batch []pendingDelivery
	for rows.Next() {
		var pd pendingDelivery
		var payload map[string]any
		if err := rows.Scan(&pd.ID, &pd.EndpointID, &pd.URL, &pd.Secret, &pd.EventType, &pd.Attempts, &payload); err != nil {
			continue
		}
		body, err := json.Marshal(map[string]any{
			"event_type": pd.EventType,
			"payload":    payload,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			continue
		}
		pd.Body = body
		batch = append(batch, pd)
	}
	if err := rows.Err(); err != nil {
		log.Printf("webhook pending scan: %v", err)
		return
	}

	for _, pd := range batch {
		d.attemptDelivery(ctx, pd)
	}
}

func (d *Dispatcher) attemptDelivery(ctx context.Context, pd pendingDelivery) {
	err := d.post(ctx, pd.URL, pd.Secret, pd.Body)
	attempts := pd.Attempts + 1
	if err == nil {
		_, _ = d.db.Exec(ctx, `
			UPDATE webhook_deliveries
			SET status = 'delivered', attempts = $2, delivered_at = NOW(), last_error = NULL
			WHERE id = $1::uuid`, pd.ID, attempts)
		return
	}
	errMsg := err.Error()
	if attempts >= d.maxAttempts {
		_, _ = d.db.Exec(ctx, `
			UPDATE webhook_deliveries
			SET status = 'dead_letter', attempts = $2, last_error = $3
			WHERE id = $1::uuid`, pd.ID, attempts, errMsg)
		log.Printf("webhook %s dead-letter after %d attempts: %v", pd.ID, attempts, err)
		return
	}
	backoff := d.retryBase * time.Duration(1<<uint(attempts-1))
	if backoff > 2*time.Minute {
		backoff = 2 * time.Minute
	}
	_, _ = d.db.Exec(ctx, `
		UPDATE webhook_deliveries
		SET attempts = $2, last_error = $3, next_retry_at = NOW() + $4::interval
		WHERE id = $1::uuid`, pd.ID, attempts, errMsg, backoff.String())
}

func (d *Dispatcher) post(ctx context.Context, url, secret string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PCMI-Event-Delivery", "1")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-PCMI-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
