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
	// Hold RLock for the entire iteration + sends. Because Unsubscribe needs
	// an exclusive Lock to close the channel, holding RLock here guarantees
	// close(ch) cannot run concurrently with ch <- evt — eliminating the
	// "send on closed channel" data race detected by -race.
	//
	// The sends are non-blocking (select/default), so RLock is never held
	// for more than a handful of microseconds per subscriber.
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers[evt.Type] {
		select {
		case ch <- evt:
		default: // non-blocking
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
