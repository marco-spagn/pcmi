//go:build integration

package event

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStreamIntegration_PublishAndConsume_EndToEnd(t *testing.T) {
	mr := startRedis(t)
	InitRedis(mr.Addr())
	t.Setenv(EnvEventBackend, BackendStreams)

	pub := NewStreamPublisher(RedisClient, StreamKey)
	consumer := NewStreamConsumer(RedisClient, StreamKey, "int-group", "int-consumer")
	if err := consumer.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var got Event
	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, evt Event, _ string) error {
			got = evt
			cancel()
			return nil
		})
	}()

	_, err := pub.Publish(context.Background(), EventMemoryStored, map[string]any{
		"tenant_id": "tenant-a",
		"path":      "root.e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	waitUntil(t, 5*time.Second, func() bool { return got.Type == EventMemoryStored })
}

func TestStreamIntegration_WorkerCrash_MessagesRecoveredAfterRestart(t *testing.T) {
	mr := startRedis(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pub := NewStreamPublisher(client, StreamKey)
	group := "crash-group"

	consumer1 := NewStreamConsumer(client, StreamKey, group, "worker-1")
	if err := consumer1.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}

	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "worker-1", Streams: []string{StreamKey, ">"},
		Count: 1, Block: time.Second,
	}).Result()
	_ = streams
	_, err = pub.Publish(context.Background(), EventMemoryUpdated, map[string]any{"tenant_id": "tid"})
	if err != nil {
		t.Fatal(err)
	}
	streams, err = client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "worker-1", Streams: []string{StreamKey, ">"},
		Count: 1, Block: 2 * time.Second,
	}).Result()
	if err != nil || len(streams) == 0 {
		t.Fatalf("first read: %v", err)
	}
	msgID := streams[0].Messages[0].ID

	mr.FastForward(2 * pendingMinIdle)

	consumer2 := NewStreamConsumer(client, StreamKey, group, "worker-2")
	var recovered bool
	consumer2.recoverPending(context.Background(), func(_ context.Context, evt Event, id string) error {
		if id == msgID {
			recovered = true
		}
		return nil
	})
	if !recovered {
		t.Fatal("message not recovered after simulated worker crash")
	}
}

func TestStreamIntegration_MultipleConsumers_LoadBalanced(t *testing.T) {
	mr := startRedis(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pub := NewStreamPublisher(client, StreamKey)
	group := "lb-group"
	for _, name := range []string{"w1", "w2"} {
		c := NewStreamConsumer(client, StreamKey, group, name)
		if err := c.EnsureGroup(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 4; i++ {
		if _, err := pub.Publish(context.Background(), EventMemoryStored, map[string]any{
			"tenant_id": "tid", "seq": i,
		}); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	seen := make(map[string]struct{})
	handler := func(_ context.Context, _ Event, streamID string) error {
		mu.Lock()
		seen[streamID] = struct{}{}
		mu.Unlock()
		return nil
	}

	var wg sync.WaitGroup
	for _, name := range []string{"w1", "w2"} {
		wg.Add(1)
		c := NewStreamConsumer(client, StreamKey, group, name)
		go func() {
			defer wg.Done()
			_ = c.Consume(ctx, handler)
		}()
	}

	waitUntil(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) == 4
	})
	cancel()
	wg.Wait()
}

func startRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		InitRedis(addr)
		return nil
	}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	return mr
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.After(timeout)
	for !fn() {
		select {
		case <-deadline:
			t.Fatal("timeout")
		default:
			time.Sleep(25 * time.Millisecond)
		}
	}
}
