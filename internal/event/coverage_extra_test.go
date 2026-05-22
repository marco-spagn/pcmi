package event

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestWorkerConsumerName_andNewWorkerStreamConsumer(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	InitRedis(mr.Addr())

	if name := WorkerConsumerName(); name == "" {
		t.Fatal("expected non-empty consumer name")
	}
	c := NewWorkerStreamConsumer()
	if c.group != WorkerConsumerGroup || c.stream != StreamKey {
		t.Fatalf("unexpected consumer: %+v", c)
	}
}

func TestSubscribeEvents_delegatesToContext(t *testing.T) {
	lockRedisTest(t)
	SetEventBackend(BackendPubSub)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	InitRedis(mr.Addr())

	ch := SubscribeEvents()
	if ch == nil {
		t.Fatal("nil channel")
	}
}

func TestEnsureGroup_idempotent(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := NewStreamConsumer(client, StreamKey, "idem-group", "c1")
	if err := c.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureGroup(context.Background()); err != nil {
		t.Fatal("second EnsureGroup:", err)
	}
}

func TestMoveToDLQ_and_metrics(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := NewStreamConsumer(client, StreamKey, "dlq-group", "c1")
	_ = c.EnsureGroup(context.Background())

	pub := NewStreamPublisher(client, StreamKey)
	id, err := pub.Publish(context.Background(), EventMemoryStored, map[string]any{"tenant_id": "tid"})
	if err != nil {
		t.Fatal(err)
	}
	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: "dlq-group", Consumer: "stale", Streams: []string{StreamKey, ">"},
		Count: 1, Block: time.Second,
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	msg := streams[0].Messages[0]
	msg.ID = id

	c.moveToDLQ(context.Background(), msg)

	entries, err := client.XRange(context.Background(), DLQStreamKey, "-", "+").Result()
	if err != nil || len(entries) == 0 {
		t.Fatalf("DLQ empty: err=%v len=%d", err, len(entries))
	}
}

func TestDecodeStreamMessage_errors(t *testing.T) {
	_, err := decodeStreamMessage(redis.XMessage{ID: "1-0", Values: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected error for missing data field")
	}
	_, err = decodeStreamMessage(redis.XMessage{
		ID: "1-0", Values: map[string]interface{}{"data": `{invalid`},
	})
	if err == nil {
		t.Fatal("expected JSON error")
	}
}

func TestSetEventBackend_values(t *testing.T) {
	SetEventBackend(BackendPubSub)
	if EventBackend() != BackendPubSub {
		t.Fatalf("got %q", EventBackend())
	}
	SetEventBackend("STREAMS")
	if EventBackend() != BackendStreams {
		t.Fatalf("got %q", EventBackend())
	}
	SetEventBackend("")
	if EventBackend() != BackendStreams {
		t.Fatalf("got %q", EventBackend())
	}
}
