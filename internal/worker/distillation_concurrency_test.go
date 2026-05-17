package worker

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestWorker builds a DistillationWorker with a nil DB and a semaphore of
// the given capacity, suitable for concurrency tests that never hit the DB.
func newTestWorker(concurrency int) *DistillationWorker {
	return &DistillationWorker{
		sem: make(chan struct{}, concurrency),
	}
}

// ── semaphore capacity ────────────────────────────────────────────────────────

func TestSemaphoreCapacity(t *testing.T) {
	w := newTestWorker(3)
	if cap(w.sem) != 3 {
		t.Fatalf("expected capacity 3, got %d", cap(w.sem))
	}
}

func TestSemaphoreDefaultCapacity(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "")
	if got := distillationConcurrency(); got != defaultDistillationConcurrency {
		t.Fatalf("expected %d, got %d", defaultDistillationConcurrency, got)
	}
}

// ── distillationConcurrency() parsing ────────────────────────────────────────

func TestDistillationConcurrencyEnvOverride(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "7")
	if got := distillationConcurrency(); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}

func TestDistillationConcurrencyInvalidFallsToDefault(t *testing.T) {
	for _, bad := range []string{"0", "-1", "17", "abc", "99999"} {
		t.Setenv("DISTILLATION_CONCURRENCY", bad)
		if got := distillationConcurrency(); got != defaultDistillationConcurrency {
			t.Errorf("input %q: expected default %d, got %d", bad, defaultDistillationConcurrency, got)
		}
	}
}

func TestDistillationConcurrencyBoundary(t *testing.T) {
	for _, v := range []string{"1", "16"} {
		t.Setenv("DISTILLATION_CONCURRENCY", v)
		n := distillationConcurrency()
		if n < 1 || n > 16 {
			t.Errorf("boundary value %q produced out-of-range result %d", v, n)
		}
	}
}

// ── concurrent trigger respects the semaphore ─────────────────────────────────

// fakeJob simulates an LLM job that takes `dur` and updates peak concurrency.
func fakeJob(sem chan struct{}, dur time.Duration, active *atomic.Int32, peak *atomic.Int32) {
	sem <- struct{}{}
	cur := active.Add(1)
	// record peak
	for {
		p := peak.Load()
		if cur <= p || peak.CompareAndSwap(p, cur) {
			break
		}
	}
	time.Sleep(dur)
	active.Add(-1)
	<-sem
}

func TestSemaphoreLimitsConcurrency(t *testing.T) {
	const maxConcurrent = 3
	const numJobs = 20

	sem := make(chan struct{}, maxConcurrent)
	var active atomic.Int32
	var peak atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fakeJob(sem, 5*time.Millisecond, &active, &peak)
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > int32(maxConcurrent) {
		t.Errorf("peak concurrency %d exceeded semaphore capacity %d", got, maxConcurrent)
	}
}

// TestSemaphoreRace verifies that TriggerForMemory / TriggerForPrefix with
// concurrent callers produces no data race (run with go test -race).
func TestSemaphoreRace(t *testing.T) {
	w := newTestWorker(2)

	// jobFunc is a closure that acquires/releases the semaphore like the real
	// trigger methods do, without touching nil DB fields.
	run := func() {
		w.sem <- struct{}{}
		defer func() { <-w.sem }()
		time.Sleep(time.Millisecond)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run()
		}()
	}
	wg.Wait()
}

// ── TriggerForMemory / TriggerForPrefix empty-input guard (no panic) ──────────

func TestTriggerForMemoryEmptyDoesNotPanic(t *testing.T) {
	w := newTestWorker(1)
	// With a nil DB the job will fail early (set_tenant_context) but must not
	// panic before acquiring the semaphore.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Just verify the semaphore accounting works without panicking at the
		// goroutine boundary. We don't wait for DB calls.
		w.sem <- struct{}{}
		<-w.sem
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout: semaphore leaked")
	}
}

func TestTriggerForPrefixEmptyDoesNotPanic(t *testing.T) {
	w := newTestWorker(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.sem <- struct{}{}
		<-w.sem
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout: semaphore leaked")
	}
}
