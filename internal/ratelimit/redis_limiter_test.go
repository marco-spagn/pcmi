package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func redisLimiter(t *testing.T, window time.Duration) (*RedisRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisRateLimiter(client, window), mr
}

func TestRedisRateLimiter_AllowsWithinLimit(t *testing.T) {
	lim, _ := redisLimiter(t, time.Minute)
	ctx := context.Background()
	key := "test:allow"

	for i := 0; i < 3; i++ {
		ok, err := lim.Allow(ctx, key, 3)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d: expected allow within limit", i+1)
		}
	}
}

func TestRedisRateLimiter_BlocksOverLimit(t *testing.T) {
	lim, _ := redisLimiter(t, time.Minute)
	ctx := context.Background()
	key := "test:block"

	for i := 0; i < 2; i++ {
		ok, err := lim.Allow(ctx, key, 2)
		if err != nil || !ok {
			t.Fatalf("request %d: expected allow, ok=%v err=%v", i+1, ok, err)
		}
	}
	ok, err := lim.Allow(ctx, key, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected block when over limit")
	}
}

func TestRedisRateLimiter_SlidingWindowAccuracy(t *testing.T) {
	window := 2 * time.Second
	lim, mr := redisLimiter(t, window)
	ctx := context.Background()
	key := "test:slide"

	for i := 0; i < 2; i++ {
		ok, err := lim.Allow(ctx, key, 2)
		if err != nil || !ok {
			t.Fatalf("warmup %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, _ := lim.Allow(ctx, key, 2)
	if ok {
		t.Fatal("expected block at window capacity")
	}

	mr.FastForward(3 * time.Second)

	ok, err := lim.Allow(ctx, key, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected allow after window slid forward")
	}
}

func TestRedisRateLimiter_MultipleInstances_SharedCounter(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	a := NewRedisRateLimiter(client, time.Minute)
	b := NewRedisRateLimiter(client, time.Minute)
	ctx := context.Background()
	key := "test:shared"

	ok, err := a.Allow(ctx, key, 2)
	if err != nil || !ok {
		t.Fatalf("instance A first: ok=%v err=%v", ok, err)
	}
	ok, err = b.Allow(ctx, key, 2)
	if err != nil || !ok {
		t.Fatalf("instance B second: ok=%v err=%v", ok, err)
	}
	ok, err = a.Allow(ctx, key, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("shared counter should block third request across instances")
	}
}

func TestRedisRateLimiter_RedisDown_FallsBackToAllow(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	lim := NewRedisRateLimiter(client, time.Minute)

	ok, err := lim.Allow(context.Background(), "test:down", 1)
	if err == nil {
		t.Fatal("expected redis error")
	}
	if !ok {
		t.Fatal("redis down should fail open and allow request")
	}
}
