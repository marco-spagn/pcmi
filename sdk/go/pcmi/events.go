package pcmi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Event is a PCMI SSE payload from GET /v1/events.
type Event struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// IngestEventRequest is the body for POST /v1/events.
type IngestEventRequest struct {
	EventType     string         `json:"event_type"`
	AgentID       string         `json:"agent_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Payload       map[string]any `json:"payload"`
}

// IngestEvent publishes a runtime event (POST /v1/events).
func (c *Client) IngestEvent(ctx context.Context, req IngestEventRequest) (map[string]any, error) {
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/events", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListEventSchemas returns registered event schemas (GET /v1/events/schemas).
func (c *Client) ListEventSchemas(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/events/schemas", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Subscribe opens an SSE stream from GET /v1/events. Events are sent on the returned
// channel until ctx is cancelled or the stream ends. The channel is closed on exit.
func (c *Client) Subscribe(ctx context.Context, opts *SubscribeOptions) (<-chan Event, error) {
	q := url.Values{}
	if opts != nil && len(opts.Types) > 0 {
		q.Set("types", strings.Join(opts.Types, ","))
	}
	path := "/v1/events"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	// SSE is long-lived; use a client without a global timeout (ctx controls lifetime).
	streamHTTP := &http.Client{Transport: c.http.Transport}
	resp, err := streamHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		return nil, parseAPIError(resp.StatusCode, data)
	}

	ch := make(chan Event, 8)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var block []string
		flush := func() {
			if len(block) == 0 {
				return
			}
			ev, ok := parseSSEBlock(block)
			block = block[:0]
			if !ok {
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		}
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := scanner.Text()
			if line == "" {
				flush()
				continue
			}
			block = append(block, line)
		}
		flush()
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			// Best-effort: stream ended with error; consumer may already have events.
			_ = err
		}
	}()

	return ch, nil
}

func parseSSEBlock(lines []string) (Event, bool) {
	var data string
	for _, line := range lines {
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data += strings.TrimPrefix(line, "data:")
			data = strings.TrimLeft(data, " ")
		}
	}
	if data == "" {
		return Event{}, false
	}
	var ev Event
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return Event{}, false
	}
	if ev.Type == "" {
		return Event{}, false
	}
	return ev, true
}

// SubscribeFunc is a convenience wrapper that invokes fn for each event until ctx ends.
func (c *Client) SubscribeFunc(ctx context.Context, opts *SubscribeOptions, fn func(Event) error) error {
	ch, err := c.Subscribe(ctx, opts)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := fn(ev); err != nil {
				return err
			}
		}
	}
}

// String returns a short debug representation of an event.
func (e Event) String() string {
	return fmt.Sprintf("Event{Type:%q}", e.Type)
}
