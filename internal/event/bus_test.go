package event

import (
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := NewBus()
	ch := bus.Subscribe(EventMemoryStored)

	bus.Publish(Event{Type: EventMemoryStored, Payload: map[string]any{"id": 1}})

	select {
	case evt := <-ch:
		if evt.Type != EventMemoryStored {
			t.Fatalf("unexpected type %s", evt.Type)
		}
		if evt.Payload["id"] != float64(1) && evt.Payload["id"] != 1 {
			t.Fatalf("unexpected payload id: %v", evt.Payload["id"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBusPublishNonBlockingWhenChannelsFull(t *testing.T) {
	bus := NewBus()
	_ = bus.Subscribe(EventMemoryStored)
	// Buffer is 10; extra publishes must not block the publisher.
	for i := 0; i < 50; i++ {
		bus.Publish(Event{Type: EventMemoryStored, Payload: map[string]any{"i": i}})
	}
}
