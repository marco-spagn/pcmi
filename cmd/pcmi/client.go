package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/version"
)

// apiClient is a deliberately small HTTP client for the handful of endpoints
// the CLI needs (the full client lives in sdk/go, a separate module).
type apiClient struct {
	base string
	key  string
	http *http.Client
}

func newAPIClient(base, key string, timeout time.Duration) *apiClient {
	return &apiClient{
		base: strings.TrimRight(strings.TrimSpace(base), "/"),
		key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: timeout},
	}
}

// apiError is a non-2xx response.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }

func (c *apiClient) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		req.Header.Set("X-API-Key", c.key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "pcmi-cli/"+version.Tag)
	return req, nil
}

// raw performs a request and returns the body of a 2xx response.
func (c *apiClient) raw(ctx context.Context, method, path string, body any) ([]byte, http.Header, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		msg := strings.TrimSpace(e.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		return nil, nil, &apiError{Status: resp.StatusCode, Message: msg}
	}
	return data, resp.Header, nil
}

// json performs a request and decodes a JSON response into out (may be nil).
func (c *apiClient) json(ctx context.Context, method, path string, body, out any) error {
	data, _, err := c.raw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

// sseEvent is one Server-Sent Event from GET /v1/events.
type sseEvent struct {
	Name string
	Data string
}

// stream reads SSE events from path until ctx ends or the server closes the
// stream, calling fn for each event (heartbeat comments are skipped).
func (c *apiClient) stream(ctx context.Context, path string, fn func(sseEvent) error) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	// No client timeout: the stream is long-lived and ctx controls its lifetime.
	resp, err := (&http.Client{Transport: c.http.Transport}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return &apiError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	var ev sseEvent
	var data []string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if ctx.Err() != nil {
				return nil
			}
			if len(data) > 0 {
				ev.Data = strings.Join(data, "\n")
				if err := fn(ev); err != nil {
					return err
				}
			}
			ev, data = sseEvent{}, nil
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			ev.Name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
