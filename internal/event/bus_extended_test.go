package event

import (
	"testing"
	"time"
)

func TestBusMultipleSubscribers(t *testing.T) {
	bus := NewBus()
	ch1 := bus.Subscribe(EventMemoryStored)
	defer bus.Unsubscribe(EventMemoryStored, ch1)
	ch2 := bus.Subscribe(EventMemoryStored)
	defer bus.Unsubscribe(EventMemoryStored, ch2)

	bus.Publish(Event{Type: EventMemoryStored, Payload: map[string]any{"id": 42}})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.Type != EventMemoryStored {
				t.Fatalf("subscriber %d: unexpected event type %s", i, evt.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

func TestBusPublishNoSubscribers(t *testing.T) {
	bus := NewBus()
	// Should not panic or block.
	bus.Publish(Event{Type: "unknown.type", Payload: map[string]any{}})
}

func TestBusPublishDifferentTypes(t *testing.T) {
	bus := NewBus()
	chStored := bus.Subscribe(EventMemoryStored)
	defer bus.Unsubscribe(EventMemoryStored, chStored)
	chUpdated := bus.Subscribe(EventMemoryUpdated)
	defer bus.Unsubscribe(EventMemoryUpdated, chUpdated)

	bus.Publish(Event{Type: EventMemoryStored, Payload: map[string]any{"path": "root.a"}})

	select {
	case evt := <-chStored:
		if evt.Type != EventMemoryStored {
			t.Fatalf("wrong type on stored channel: %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("stored channel timed out")
	}

	// chUpdated must not have received anything
	select {
	case evt := <-chUpdated:
		t.Fatalf("updated channel received unexpected event: %v", evt)
	default:
	}
}

func TestBusSubscribeReturnsDifferentChannels(t *testing.T) {
	bus := NewBus()
	ch1 := bus.Subscribe(EventKnowledgeDistilled)
	defer bus.Unsubscribe(EventKnowledgeDistilled, ch1)
	ch2 := bus.Subscribe(EventKnowledgeDistilled)
	defer bus.Unsubscribe(EventKnowledgeDistilled, ch2)
	if ch1 == ch2 {
		t.Fatal("Subscribe must return distinct channels for each subscriber")
	}
}

func TestBusPublishNonBlocking(t *testing.T) {
	bus := NewBus()
	ch := bus.Subscribe(EventMemoryRefineRequested)
	defer bus.Unsubscribe(EventMemoryRefineRequested, ch)

	// Fill the buffer (capacity 10)
	for i := 0; i < 20; i++ {
		bus.Publish(Event{Type: EventMemoryRefineRequested, Payload: map[string]any{"i": i}})
	}
	// Must not block; channel buffer is 10, extra publishes are dropped silently.
	_ = ch
}

