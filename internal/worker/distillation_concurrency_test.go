package worker

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestWorker(concurrency int) *DistillationWorker {
	return &DistillationWorker{
		sem: make(chan struct{}, concurrency),
	}
}

func TestSemaphoreCapacity(t *testing.T) {
	w := newTestWorker(3)
	if cap(w.sem) != 3 {
		t.Fatalf("expected capacity 3, got %d", cap(w.sem))
	}
}

func TestSemaphoreDefaultCapacity(t *testing.T) {
	if got := distillationConcurrencyFrom(0); got != defaultDistillationConcurrency {
		t.Fatalf("expected %d, got %d", defaultDistillationConcurrency, got)
	}
}

func TestDistillationConcurrencyEnvOverride(t *testing.T) {
	if got := distillationConcurrencyFrom(7); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}

func TestDistillationConcurrencyInvalidFallsToDefault(t *testing.T) {
	for _, bad := range []int{0, -1, 17, 99999} {
		if got := distillationConcurrencyFrom(bad); got != defaultDistillationConcurrency {
			t.Errorf("input %d: expected default %d, got %d", bad, defaultDistillationConcurrency, got)
		}
	}
}

func fakeJob(sem chan struct{}, dur time.Duration, active *atomic.Int32, peak *atomic.Int32) {
	sem <- struct{}{}
	cur := active.Add(1)
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

func TestSemaphoreRace(t *testing.T) {
	w := newTestWorker(2)

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

func TestTriggerForMemoryEmptyDoesNotPanic(t *testing.T) {
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
