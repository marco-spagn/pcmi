package pcmi

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is returned when PCMI responds with a non-success HTTP status.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("pcmi api: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("pcmi api: HTTP %d: %s", e.StatusCode, e.Message)
}

// IsRetryable reports whether the caller should retry the request.
func (e *APIError) IsRetryable() bool {
	return e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode == http.StatusServiceUnavailable ||
		e.StatusCode == http.StatusGatewayTimeout
}

// ErrInvalidConfig indicates missing or invalid client configuration.
var ErrInvalidConfig = errors.New("pcmi: invalid client configuration")
