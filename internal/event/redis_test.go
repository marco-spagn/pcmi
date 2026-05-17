package event

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestPublishEventRoundTrip(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	InitRedis(mr.Addr())

	ch := SubscribeEvents()

	err = PublishEvent(EventMemoryStored, map[string]any{
		"id":        1,
		"tenant_id": "tid",
		"path":      "root.test",
		"version":   1,
	})
	if err != nil {
		t.Fatalf("PublishEvent failed: %v", err)
	}

	select {
	case evt := <-ch:
		if evt.Type != EventMemoryStored {
			t.Fatalf("expected %s, got %s", EventMemoryStored, evt.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event from Redis pub/sub")
	}
}

func TestPublishMultipleEvents(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	InitRedis(mr.Addr())

	types := []string{
		EventMemoryStored,
		EventMemoryUpdated,
		EventKnowledgeDistilled,
	}
	for _, et := range types {
		if err := PublishEvent(et, map[string]any{"tenant_id": "tid"}); err != nil {
			t.Fatalf("PublishEvent(%s) failed: %v", et, err)
		}
	}
}

func TestWebhookNotifierCalled(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	InitRedis(mr.Addr())

	var called bool
	SetWebhookNotifier(func(tenantID, eventType string, payload map[string]any) {
		called = true
	})
	defer SetWebhookNotifier(nil)

	_ = PublishEvent(EventMemoryStored, map[string]any{
		"tenant_id": "tid",
		"id":        1,
		"path":      "root.a",
		"version":   1,
	})
	// Give the goroutine a moment.
	time.Sleep(50 * time.Millisecond)
	if !called {
		t.Fatal("webhook notifier was not called after PublishEvent")
	}
}
