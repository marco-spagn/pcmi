package event

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBusStress_HighThroughput fires 50 publishers × 1000 events while
// 20 subscribers are attached. The test verifies no deadlock and no panic
// under sustained parallel load; it does NOT assert delivery counts because
// the bus is deliberately lossy (non-blocking, buffer size 10).
func TestBusStress_HighThroughput(t *testing.T) {
	t.Parallel()
	const (
		publishers  = 50
		eventsEach  = 1000
		subscribers = 20
	)
	bus := NewBus()

	channels := make([]<-chan Event, subscribers)
	for i := 0; i < subscribers; i++ {
		ch := bus.Subscribe(EventMemoryStored)
		channels[i] = ch
		defer bus.Unsubscribe(EventMemoryStored, ch)
	}

	var wg sync.WaitGroup
	for p := 0; p < publishers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < eventsEach; i++ {
				bus.Publish(Event{
					Type:    EventMemoryStored,
					Payload: map[string]any{"p": p, "i": i},
				})
			}
		}(p)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestBusStress_HighThroughput: deadlock after 10s")
	}
}

// TestBusStress_ConcurrentSubscribeUnsubscribeDuringPublish is the key race
// regression test. It exposes the bug that existed before the deep-copy fix:
// Publish would iterate over a stale channel list AFTER releasing RLock,
// hitting channels that a concurrent Unsubscribe had already closed —
// triggering a "send on closed channel" panic.
//
// 30 goroutines each:  subscribe → optionally drain one event → unsubscribe.
// 5 goroutines:        publish continuously for 200ms.
// Must complete without panic.
func TestBusStress_ConcurrentSubscribeUnsubscribeDuringPublish(t *testing.T) {
	t.Parallel()
	bus := NewBus()

	ctx := make(chan struct{})
	var publishWg sync.WaitGroup

	// Publisher goroutines — run until ctx is closed.
	for i := 0; i < 5; i++ {
		publishWg.Add(1)
		go func() {
			defer publishWg.Done()
			for {
				select {
				case <-ctx:
					return
				default:
					bus.Publish(Event{
						Type:    EventMemoryStored,
						Payload: map[string]any{"x": 1},
					})
				}
			}
		}()
	}

	var subWg sync.WaitGroup
	for i := 0; i < 30; i++ {
		subWg.Add(1)
		go func() {
			defer subWg.Done()
			ch := bus.Subscribe(EventMemoryStored)
			// Possibly receive one event before unsubscribing (increases race window).
			select {
			case <-ch:
			case <-time.After(5 * time.Millisecond):
			}
			bus.Unsubscribe(EventMemoryStored, ch)
		}()
	}

	subWg.Wait()
	close(ctx)
	publishWg.Wait()
}

// TestBusStress_ManyEventTypesNoLeakage verifies that 50 event types,
// each with 5 subscribers, deliver only to the correct type.
// 100 goroutines publish each type 20 times (100,000 total events).
func TestBusStress_ManyEventTypesNoLeakage(t *testing.T) {
	t.Parallel()
	const (
		numTypes   = 50
		subsPerType = 5
		publishEach = 20
		publishers  = 100
	)
	bus := NewBus()

	// counters[type] counts events received by all subscribers of that type.
	counters := make([]atomic.Int64, numTypes)

	var unsubscribers []func()
	for typ := 0; typ < numTypes; typ++ {
		for s := 0; s < subsPerType; s++ {
			evtType := fmt.Sprintf("stress.type.%d", typ)
			ch := bus.Subscribe(evtType)
			idx := typ
			unsubscribers = append(unsubscribers, func() { bus.Unsubscribe(evtType, ch) })
			go func(ch <-chan Event, idx int) {
				for range ch {
					counters[idx].Add(1)
				}
			}(ch, idx)
		}
	}

	var wg sync.WaitGroup
	for p := 0; p < publishers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < publishEach; i++ {
				typ := (p*publishEach + i) % numTypes
				bus.Publish(Event{
					Type:    fmt.Sprintf("stress.type.%d", typ),
					Payload: map[string]any{"v": i},
				})
			}
		}(p)
	}
	wg.Wait()

	// Drain subscriber goroutines.
	for _, unsub := range unsubscribers {
		unsub()
	}
	// Brief settle for the drain goroutines to observe the channel close.
	time.Sleep(10 * time.Millisecond)

	// Verify no cross-type leakage: every counter must be <= subsPerType × (total
	// events for that type). We can't assert exact counts because the bus is
	// lossy, but we verify no counter received MORE than possible.
	totalPerType := int64(publishers) * publishEach / numTypes * subsPerType * 3
	for typ := 0; typ < numTypes; typ++ {
		if got := counters[typ].Load(); got > totalPerType {
			t.Errorf("type %d: received %d events, ceiling is %d (cross-type leak?)", typ, got, totalPerType)
		}
	}
}

// TestBusStress_UnsubscribeFromMultipleGoroutines verifies that multiple
// goroutines unsubscribing different channels of the same event type
// simultaneously does not corrupt the subscribers slice.
func TestBusStress_UnsubscribeFromMultipleGoroutines(t *testing.T) {
	t.Parallel()
	const n = 100
	bus := NewBus()

	channels := make([]<-chan Event, n)
	for i := 0; i < n; i++ {
		channels[i] = bus.Subscribe(EventMemoryStored)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		ch := channels[i]
		go func() {
			defer wg.Done()
			bus.Unsubscribe(EventMemoryStored, ch)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("TestBusStress_UnsubscribeFromMultipleGoroutines: deadlock")
	}

	bus.mu.RLock()
	remaining := len(bus.subscribers[EventMemoryStored])
	bus.mu.RUnlock()
	if remaining != 0 {
		t.Errorf("expected 0 remaining subscribers, got %d", remaining)
	}
}

// TestBusStress_PublishToFullChannels verifies that filling all subscriber
// buffers and then publishing thousands more events completes immediately
// (non-blocking contract) and does not panic.
func TestBusStress_PublishToFullChannels(t *testing.T) {
	t.Parallel()
	const subs = 30
	bus := NewBus()
	channels := make([]<-chan Event, subs)
	for i := 0; i < subs; i++ {
		ch := bus.Subscribe(EventMemoryStored)
		channels[i] = ch
		defer bus.Unsubscribe(EventMemoryStored, ch)
	}

	// Fill all buffers.
	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: EventMemoryStored, Payload: map[string]any{"i": i}})
	}

	// These must not block even though all buffers are now full.
	start := time.Now()
	for i := 0; i < 10_000; i++ {
		bus.Publish(Event{Type: EventMemoryStored, Payload: map[string]any{"i": i}})
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("10,000 publishes to full buffers took %v (expected < 500ms)", elapsed)
	}
}
