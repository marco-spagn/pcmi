package event

func lockRedisTest(t testingT) {
	t.Helper()
	TestLockRedis(t)
	SetEventBackend(BackendStreams)
}

func closeRedisTest(t testingT, mr interface{ Close() }) {
	t.Helper()
	if RedisClient != nil {
		_ = RedisClient.Close()
		// Do NOT set RedisClient = nil — streamSubscribe goroutines
		// may still be reading it (race detector). The closed connection
		// will cause them to exit gracefully.
	}
	if mr != nil {
		mr.Close()
	}
}

type testingT interface {
	Helper()
	Cleanup(func())
}
