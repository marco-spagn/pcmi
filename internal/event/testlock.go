package event

import "sync"

var redisTestMu sync.Mutex

// TestLockRedis serializes tests that replace the package-level RedisClient.
func TestLockRedis(t interface {
	Helper()
	Cleanup(func())
}) {
	t.Helper()
	redisTestMu.Lock()
	t.Cleanup(func() { redisTestMu.Unlock() })
}
