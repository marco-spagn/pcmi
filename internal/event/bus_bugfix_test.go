package event

import (
	"testing"
	"time"
)

// TestBus_UnsubscribeRemovesChannel verifies BUG-FIX-6:
// Unsubscribe must remove the channel from the bus and close it.
func TestBus_UnsubscribeRemovesChannel(t *testing.T) {
	b := NewBus()
	ch := b.Subscribe("test.event")

	// Unsubscribe should not panic and must drain the map entry.
	b.Unsubscribe("test.event", ch)

	b.mu.RLock()
	remaining := len(b.subscribers["test.event"])
	b.mu.RUnlock()

	if remaining != 0 {
		t.Errorf("expected 0 subscribers after Unsubscribe, got %d", remaining)
	}

	// Channel must be closed — a receive should unblock immediately.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed (ok=false), got a value")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("channel not closed after Unsubscribe — timeout")
	}
}

// TestBus_UnsubscribeEmptyMapEntry verifies the map key is removed when the
// last subscriber unsubscribes (prevents map growth).
func TestBus_UnsubscribeEmptyMapEntry(t *testing.T) {
	b := NewBus()
	ch1 := b.Subscribe("evt")
	ch2 := b.Subscribe("evt")

	b.Unsubscribe("evt", ch1)
	b.mu.RLock()
	_, ok1 := b.subscribers["evt"]
	b.mu.RUnlock()
	if !ok1 {
		t.Error("map entry should remain after first of two unsubscribes")
	}

	b.Unsubscribe("evt", ch2)
	b.mu.RLock()
	_, ok2 := b.subscribers["evt"]
	b.mu.RUnlock()
	if ok2 {
		t.Error("map entry should be deleted after last subscriber unsubscribes")
	}
}

// TestBus_UnsubscribeUnknownChannelNoOp verifies no panic on unknown channel.
func TestBus_UnsubscribeUnknownChannelNoOp(t *testing.T) {
	b := NewBus()
	orphan := make(chan Event, 1)
	b.Unsubscribe("not.registered", orphan) // must not panic
}

// TestBus_PublishAfterUnsubscribeNoDeadlock verifies Publish is safe after
// the channel is removed (previously could deliver to a closed channel).
func TestBus_PublishAfterUnsubscribeNoDeadlock(t *testing.T) {
	b := NewBus()
	ch := b.Subscribe("my.event")
	b.Unsubscribe("my.event", ch)

	// Should complete immediately without panic.
	done := make(chan struct{})
	go func() {
		b.Publish(Event{Type: "my.event", Payload: nil})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Error("Publish blocked after Unsubscribe")
	}
}

// TestBus_MultipleSubscribersOneUnsubscribes verifies remaining subscriber
// still receives events after a peer unsubscribes.
func TestBus_MultipleSubscribersOneUnsubscribes(t *testing.T) {
	b := NewBus()
	chA := b.Subscribe("data")
	chB := b.Subscribe("data")

	b.Unsubscribe("data", chA) // A leaves; B should still receive

	b.Publish(Event{Type: "data", Payload: map[string]any{"k": "v"}})

	select {
	case evt := <-chB:
		if evt.Type != "data" {
			t.Errorf("unexpected event type %q", evt.Type)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("chB did not receive event after chA unsubscribed")
	}
}
