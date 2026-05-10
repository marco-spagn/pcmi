package event

import (
	"sync"
)

type Event struct {
	Type    string
	Payload map[string]any
}

type Bus struct {
	subscribers map[string][]chan Event
	mu          sync.RWMutex
}

var GlobalBus = NewBus()

func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[string][]chan Event),
	}
}

func (b *Bus) Publish(evt Event) {
	b.mu.RLock()
	channels := b.subscribers[evt.Type]
	b.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- evt:
		default:
			// non bloccante
		}
	}
}

func (b *Bus) Subscribe(eventType string) <-chan Event {
	ch := make(chan Event, 10)

	b.mu.Lock()
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	b.mu.Unlock()

	return ch
}
