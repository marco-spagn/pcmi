package event

import "sync"

// redisTestMu serializes tests that share the package-level RedisClient.
var redisTestMu sync.Mutex

func lockRedisTest(t testingT) {
	t.Helper()
	redisTestMu.Lock()
	SetEventBackend(BackendStreams)
	t.Cleanup(func() { redisTestMu.Unlock() })
}

func closeRedisTest(t testingT, mr interface{ Close() }) {
	t.Helper()
	if RedisClient != nil {
		_ = RedisClient.Close()
		RedisClient = nil
	}
	if mr != nil {
		mr.Close()
	}
}

type testingT interface {
	Helper()
	Cleanup(func())
}
