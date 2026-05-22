package event

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestPublishEventRoundTrip(t *testing.T) {
	lockRedisTest(t)
	t.Setenv(EnvEventBackend, BackendPubSub)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}

	InitRedis(mr.Addr())

	subCtx, subCancel := context.WithCancel(context.Background())
	defer func() {
		subCancel()
		closeRedisTest(t, mr)
	}()
	ch := SubscribeEventsContext(subCtx)

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

func TestPublishStreamsRoundTrip(t *testing.T) {
	lockRedisTest(t)
	t.Setenv(EnvEventBackend, BackendStreams)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}

	InitRedis(mr.Addr())

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer func() {
		streamCancel()
		closeRedisTest(t, mr)
	}()
	ch := SubscribeEventsContext(streamCtx)
	// Let the tailing XREAD start before XADD (avoids missing events when the scheduler runs Publish first).
	time.Sleep(50 * time.Millisecond)

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
		if evt.Payload[PayloadKeyStreamID] == nil || evt.Payload[PayloadKeyStreamID] == "" {
			t.Fatal("expected stream_id in payload")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event from Redis stream")
	}
}

func TestPublishMultipleEvents(t *testing.T) {
	lockRedisTest(t)
	t.Setenv(EnvEventBackend, BackendPubSub)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()

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
	lockRedisTest(t)
	t.Setenv(EnvEventBackend, BackendPubSub)
	mr, _ := miniredis.Run()
	defer func() { closeRedisTest(t, mr) }()
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

func TestSubscribeEventsContext_cancelledParent(t *testing.T) {
	lockRedisTest(t)
	t.Setenv(EnvEventBackend, BackendPubSub)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	InitRedis(mr.Addr())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := SubscribeEventsContext(ctx)
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel when parent ctx cancelled before subscribe completes")
	}
}

func TestPublishEvent_noWebhookWithoutTenantID(t *testing.T) {
	lockRedisTest(t)
	t.Setenv(EnvEventBackend, BackendPubSub)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	InitRedis(mr.Addr())

	var webhookCalls int
	SetWebhookNotifier(func(string, string, map[string]any) { webhookCalls++ })
	defer SetWebhookNotifier(nil)

	if err := PublishEvent(EventMemoryStored, map[string]any{"id": 1}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if webhookCalls != 0 {
		t.Fatalf("webhook calls=%d want 0 without tenant_id", webhookCalls)
	}
}
