package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestKey_emptyRoleAndClient(t *testing.T) {
	got := Key("", "")
	if got != "pcmi:ratelimit:default:anon" {
		t.Fatalf("got %q", got)
	}
}

func TestLimitFromRPM_scaling(t *testing.T) {
	if got := LimitFromRPM(120, 30); got != 60 {
		t.Fatalf("30s window: got %d want 60", got)
	}
	if got := LimitFromRPM(0, 60); got != 1 {
		t.Fatalf("zero rpm: got %d want 1", got)
	}
	// windowSecs < 1 is treated as 60s, so 10 RPM over 60s → 10 requests.
	if got := LimitFromRPM(10, 0); got != 10 {
		t.Fatalf("zero window defaults to 60s: got %d want 10", got)
	}
}

func TestParseWindowSecs_nonPositive(t *testing.T) {
	if got := ParseWindowSecs(0); got != time.Minute {
		t.Fatalf("got %v want 1m", got)
	}
	if got := ParseWindowSecs(-5); got != time.Minute {
		t.Fatalf("got %v want 1m", got)
	}
}

func TestRedisRateLimiter_nilReceiver(t *testing.T) {
	var lim *RedisRateLimiter
	ok, err := lim.Allow(context.Background(), "k", 10)
	if err != nil || !ok {
		t.Fatalf("nil limiter should fail open: ok=%v err=%v", ok, err)
	}
	if lim.Window() != 0 {
		t.Fatalf("window=%v", lim.Window())
	}
}

func TestRedisRateLimiter_zeroLimit(t *testing.T) {
	lim, _ := redisLimiter(t, time.Minute)
	ok, err := lim.Allow(context.Background(), "zero-limit", 0)
	if err != nil || !ok {
		t.Fatalf("limit<1 should allow: ok=%v err=%v", ok, err)
	}
}
