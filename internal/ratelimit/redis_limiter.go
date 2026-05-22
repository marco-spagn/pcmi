// Package ratelimit provides distributed request rate limiting backed by Redis.
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowLua atomically trims expired entries, records the request, and
// returns 1 when allowed or 0 when the window is full.
const slidingWindowLua = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, 0, now - window_ms)
redis.call('ZADD', key, now, member)
local count = redis.call('ZCARD', key)
redis.call('PEXPIRE', key, window_ms)
if count > limit then
  return 0
end
return 1
`

// RedisRateLimiter enforces a sliding-window request cap per key using Redis ZSETs.
type RedisRateLimiter struct {
	client *redis.Client
	window time.Duration
	script *redis.Script
}

// NewRedisRateLimiter returns a limiter that shares counts across processes via Redis.
func NewRedisRateLimiter(client *redis.Client, window time.Duration) *RedisRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &RedisRateLimiter{
		client: client,
		window: window,
		script: redis.NewScript(slidingWindowLua),
	}
}

// Allow reports whether a request for key should proceed under the given limit.
// When Redis is unavailable, it fails open (allows the request).
func (l *RedisRateLimiter) Allow(ctx context.Context, key string, limit int) (bool, error) {
	if l == nil || l.client == nil || limit < 1 {
		return true, nil
	}
	now := time.Now().UnixMilli()
	windowMs := l.window.Milliseconds()
	member := fmt.Sprintf("%d:%d", now, time.Now().UnixNano())

	res, err := l.script.Run(ctx, l.client, []string{key},
		now, windowMs, limit, member,
	).Int()
	if err != nil {
		return true, err
	}
	return res == 1, nil
}

// Window returns the configured sliding window duration.
func (l *RedisRateLimiter) Window() time.Duration {
	if l == nil {
		return 0
	}
	return l.window
}

// Key builds a stable Redis key for a role and client identifier.
func Key(role, clientID string) string {
	if role == "" {
		role = "default"
	}
	if clientID == "" {
		clientID = "anon"
	}
	return "pcmi:ratelimit:" + role + ":" + clientID
}

// ParseWindowSecs converts window seconds from config to a duration.
func ParseWindowSecs(secs int) time.Duration {
	if secs < 1 {
		secs = 60
	}
	return time.Duration(secs) * time.Second
}

// LimitFromRPM maps requests-per-minute to a cap for a window of windowSecs seconds.
func LimitFromRPM(rpm, windowSecs int) int {
	if rpm < 1 {
		rpm = 1
	}
	if windowSecs < 1 {
		windowSecs = 60
	}
	// RPM is defined over 60s; scale linearly for shorter windows in tests.
	limit := (rpm * windowSecs) / 60
	if limit < 1 {
		limit = 1
	}
	return limit
}

// MemberCount returns the number of entries in the sliding window (test helper).
func (l *RedisRateLimiter) MemberCount(ctx context.Context, key string) (int64, error) {
	if l == nil || l.client == nil {
		return 0, nil
	}
	now := time.Now().UnixMilli()
	windowMs := l.window.Milliseconds()
	if err := l.client.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(now-windowMs, 10)).Err(); err != nil {
		return 0, err
	}
	return l.client.ZCard(ctx, key).Result()
}
