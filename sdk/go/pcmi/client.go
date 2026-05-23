package pcmi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client is a thin HTTP client for the PCMI REST API.
type Client struct {
	baseURL      string
	apiKey       string
	http         *http.Client
	timeout      time.Duration
	maxRetries   int
	retryBackoff time.Duration
}

// NewClient creates a PCMI HTTP client. baseURL and apiKey must be non-empty.
func NewClient(baseURL, apiKey string, opts ...ClientOption) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return nil, ErrInvalidConfig
	}

	c := &Client{
		baseURL:      baseURL,
		apiKey:       apiKey,
		timeout:      defaultTimeout,
		maxRetries:   defaultMaxRetries,
		retryBackoff: defaultRetryBackoff,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: c.timeout}
	}
	return c, nil
}

// BaseURL returns the configured API base URL (without trailing slash).
func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.retryBackoff * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		var rdr io.Reader
		if len(payload) > 0 {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("X-API-Key", c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if isNetworkRetryable(err) && attempt < c.maxRetries {
				continue
			}
			return err
		}

		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < c.maxRetries {
				continue
			}
			return readErr
		}

		if resp.StatusCode >= 400 {
			apiErr := parseAPIError(resp.StatusCode, data)
			lastErr = apiErr
			if apiErr.IsRetryable() && attempt < c.maxRetries {
				continue
			}
			return apiErr
		}

		if out == nil || len(data) == 0 {
			return nil
		}
		return json.Unmarshal(data, out)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("pcmi: request failed after retries")
}

func parseAPIError(status int, data []byte) *APIError {
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &errBody)
	msg := strings.TrimSpace(errBody.Error)
	if msg == "" {
		msg = strings.TrimSpace(string(data))
	}
	return &APIError{StatusCode: status, Message: msg}
}

func isNetworkRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}

func applyStoreOpts(body map[string]any, opts *StoreOptions) {
	if opts == nil {
		return
	}
	if len(opts.Tags) > 0 {
		body["tags"] = opts.Tags
	}
	if len(opts.Embedding) > 0 {
		body["embedding"] = opts.Embedding
	}
	if opts.EmbeddingModel != "" {
		body["embedding_model"] = opts.EmbeddingModel
	}
	if opts.EmbeddingSpace != "" {
		body["embedding_space"] = opts.EmbeddingSpace
	}
	if opts.SourceAgentID != "" {
		body["source_agent_id"] = opts.SourceAgentID
	}
	if opts.EncryptContent != nil {
		body["encrypt_content"] = *opts.EncryptContent
	}
	if opts.ExpiresAt != "" {
		body["expires_at"] = opts.ExpiresAt
	}
	if opts.Importance != nil {
		body["importance"] = *opts.Importance
	}
}

func applyRetrieveOpts(body map[string]any, opts *RetrieveOptions) {
	if opts == nil {
		return
	}
	if opts.AsOf != "" {
		body["as_of"] = opts.AsOf
	}
	if opts.SourceAgentID != "" {
		body["source_agent_id"] = opts.SourceAgentID
	}
	if opts.EmbeddingSpace != "" {
		body["embedding_space"] = opts.EmbeddingSpace
	}
	if len(opts.Tags) > 0 {
		body["tags"] = opts.Tags
	}
	if opts.TagsMatch != "" {
		body["tags_match"] = opts.TagsMatch
	}
	if opts.DecayEnabled != nil {
		body["decay_enabled"] = *opts.DecayEnabled
	}
}
