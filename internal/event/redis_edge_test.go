package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPublishEvent_nilRedisClient(t *testing.T) {
	lockRedisTest(t)
	RedisClient = nil
	SetEventBackend(BackendPubSub)

	err := PublishEvent(EventMemoryStored, map[string]any{"tenant_id": "tid"})
	if !errors.Is(err, ErrRedisNotInitialized) {
		t.Fatalf("got err=%v want ErrRedisNotInitialized", err)
	}

	SetEventBackend(BackendStreams)
	err = PublishEvent(EventMemoryStored, map[string]any{"tenant_id": "tid"})
	if !errors.Is(err, ErrRedisNotInitialized) {
		t.Fatalf("streams: got err=%v want ErrRedisNotInitialized", err)
	}
}

func TestSubscribeEventsContext_nilRedisClient(t *testing.T) {
	lockRedisTest(t)
	RedisClient = nil

	for _, backend := range []string{BackendPubSub, BackendStreams} {
		SetEventBackend(backend)
		ch := SubscribeEventsContext(context.Background())
		_, ok := <-ch
		if ok {
			t.Fatalf("backend %s: expected closed channel", backend)
		}
	}
}

func TestPubSubSubscribe_skipsMalformedJSON(t *testing.T) {
	lockRedisTest(t)
	SetEventBackend(BackendPubSub)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	InitRedis(mr.Addr())

	subCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch := SubscribeEventsContext(subCtx)
	time.Sleep(30 * time.Millisecond)

	// Invalid JSON must not crash the subscriber goroutine.
	if err := RedisClient.Publish(context.Background(), "memory_events", "not-json").Err(); err != nil {
		t.Fatal(err)
	}
	if err := PublishEvent(EventMemoryStored, map[string]any{"tenant_id": "tid", "path": "root.ok"}); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-ch:
		if evt.Type != EventMemoryStored {
			t.Fatalf("got type %q", evt.Type)
		}
	case <-subCtx.Done():
		t.Fatal("timed out waiting for valid event after malformed publish")
	}
}

func TestDecodeStreamMessage_typeFromStreamField(t *testing.T) {
	evt, err := decodeStreamMessage(redis.XMessage{
		ID: "1-0",
		Values: map[string]interface{}{
			streamFieldType: EventKnowledgeDistilled,
			streamFieldData: `{"payload":{"tenant_id":"tid"}}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if evt.Type != EventKnowledgeDistilled {
		t.Fatalf("type=%q", evt.Type)
	}
	if evt.Payload["tenant_id"] != "tid" {
		t.Fatalf("payload=%v", evt.Payload)
	}
	if evt.Payload[PayloadKeyStreamID] != "1-0" {
		t.Fatalf("stream_id=%v", evt.Payload[PayloadKeyStreamID])
	}
}
