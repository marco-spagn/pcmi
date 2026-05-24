package pcmi

import (
	"net/http"
	"time"
)

const (
	defaultTimeout      = 60 * time.Second
	defaultMaxRetries   = 3
	defaultRetryBackoff = 200 * time.Millisecond
)

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom http.Client (timeout on the client is respected).
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) {
		if c != nil {
			cl.http = c
		}
	}
}

// WithTimeout sets the per-request timeout when no custom http.Client is supplied.
func WithTimeout(d time.Duration) ClientOption {
	return func(cl *Client) {
		if d > 0 {
			cl.timeout = d
		}
	}
}

// WithRetries sets how many times transient network/API failures are retried.
func WithRetries(n int) ClientOption {
	return func(cl *Client) {
		if n >= 0 {
			cl.maxRetries = n
		}
	}
}

// WithRetryBackoff sets the base delay between retries (doubled each attempt).
func WithRetryBackoff(d time.Duration) ClientOption {
	return func(cl *Client) {
		if d > 0 {
			cl.retryBackoff = d
		}
	}
}

// StoreOptions are optional fields for POST /v1/memories.
type StoreOptions struct {
	Tags           []string
	Embedding      []float64
	EmbeddingModel string
	EmbeddingSpace string
	SourceAgentID  string
	EncryptContent *bool
	ExpiresAt      string
	Importance     *float64
}

// RetrieveOptions are optional fields for POST /v1/retrieve.
type RetrieveOptions struct {
	AsOf           string
	SourceAgentID  string
	EmbeddingSpace string
	Tags           []string
	TagsMatch      string // "any" | "all"
	DecayEnabled   *bool
}

// SubscribeOptions filter SSE events from GET /v1/events.
type SubscribeOptions struct {
	Types []string
}
