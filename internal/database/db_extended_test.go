package database

import (
	"testing"
)

func TestNew_invalidURL_panics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from New() with invalid URL, but none occurred")
		}
	}()

	// "not-a-valid-postgres-url" causes pgxpool.New to return an error,
	// which triggers the panic inside New().
	_ = New("not-a-valid-postgres-url")
	t.Error("expected panic was not triggered")
}
