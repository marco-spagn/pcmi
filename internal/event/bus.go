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

// Unsubscribe removes ch from the subscriber list for eventType and closes it.
// BUG-FIX-6: without Unsubscribe, every call to Subscribe leaks a channel that
// accumulates in the GlobalBus map for the lifetime of the process. In K8s
// rolling deploys where workers restart frequently, this causes unbounded
// memory growth. Callers should defer Unsubscribe(type, ch) after Subscribe.
func (b *Bus) Unsubscribe(eventType string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[eventType]
	for i, s := range subs {
		if s == ch {
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			close(s)
			break
		}
	}
	// Clean up empty slice to avoid map growth.
	if len(b.subscribers[eventType]) == 0 {
		delete(b.subscribers, eventType)
	}
}
