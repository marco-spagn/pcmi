package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStreamPublisher_Publish_Success(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pub := NewStreamPublisher(client, StreamKey)

	id, err := pub.Publish(context.Background(), EventMemoryStored, map[string]any{
		"tenant_id": "tid",
		"path":      "root.test",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty stream ID")
	}
	entries, err := client.XRange(context.Background(), StreamKey, id, id).Result()
	if err != nil || len(entries) != 1 {
		t.Fatalf("XRange: err=%v len=%d", err, len(entries))
	}
}

func TestStreamPublisher_Publish_RedisDown_ReturnsError(t *testing.T) {
	lockRedisTest(t)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	pub := NewStreamPublisher(client, StreamKey)

	_, err := pub.Publish(context.Background(), EventMemoryStored, map[string]any{"tenant_id": "tid"})
	if err == nil {
		t.Fatal("expected error when Redis is unreachable")
	}
}

func TestStreamConsumer_Consume_AcksOnSuccess(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pub := NewStreamPublisher(client, StreamKey)
	consumer := NewStreamConsumer(client, StreamKey, "test-group", "c1")
	if err := consumer.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan string, 1)
	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, _ Event, streamID string) error {
			done <- streamID
			return nil
		})
	}()

	_, err = pub.Publish(context.Background(), EventMemoryStored, map[string]any{"tenant_id": "tid"})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for consumer")
	}
	cancel()
	time.Sleep(50 * time.Millisecond)

	pending, err := client.XPending(context.Background(), StreamKey, "test-group").Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected 0 pending after ACK, got %d", pending.Count)
	}
}

func TestStreamConsumer_Consume_NoAckOnProcessingError(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pub := NewStreamPublisher(client, StreamKey)
	consumer := NewStreamConsumer(client, StreamKey, "test-group", "c1")
	if err := consumer.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}

	id, err := pub.Publish(context.Background(), EventMemoryStored, map[string]any{"tenant_id": "tid"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = consumer.Consume(ctx, func(_ context.Context, _ Event, _ string) error {
		return errors.New("processing failed")
	})
	if err == nil {
		t.Fatal("expected context deadline")
	}

	pending, err := client.XPending(context.Background(), StreamKey, "test-group").Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count == 0 {
		t.Fatal("expected pending message when handler returns error")
	}
	_ = id
}

func TestStreamConsumer_PendingRecovery_ReprocessesStuckMessages(t *testing.T) {
	lockRedisTest(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeRedisTest(t, mr) }()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pub := NewStreamPublisher(client, StreamKey)
	group := "recover-group"
	consumer := NewStreamConsumer(client, StreamKey, group, "c1")
	if err := consumer.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, err = pub.Publish(context.Background(), EventMemoryUpdated, map[string]any{"tenant_id": "tid"})
	if err != nil {
		t.Fatal(err)
	}
	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "stale", Streams: []string{StreamKey, ">"},
		Count: 1, Block: 2 * time.Second,
	}).Result()
	if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
		t.Fatalf("XReadGroup: %v", err)
	}
	msgID := streams[0].Messages[0].ID

	mr.FastForward(2 * pendingMinIdle)

	var recovered bool
	consumer.recoverPending(context.Background(), func(_ context.Context, evt Event, streamID string) error {
		if streamID == msgID && evt.Type == EventMemoryUpdated {
			recovered = true
		}
		return nil
	})

	if !recovered {
		t.Fatal("pending recovery did not reprocess stuck message")
	}
}
