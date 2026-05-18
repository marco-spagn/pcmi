package event

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

var (
	RedisClient *redis.Client
	ctx         = context.Background()

	webhookNotify func(tenantID, eventType string, payload map[string]any)
)

// SetWebhookNotifier registers a callback for outbound webhook delivery (best-effort).
func SetWebhookNotifier(fn func(tenantID, eventType string, payload map[string]any)) {
	webhookNotify = fn
}

// InitRedis initializes Redis connection
func InitRedis(addr string) {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "",
		DB:       0,
	})

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis")
}

// PublishEvent publishes an event to Redis
func PublishEvent(eventType string, payload map[string]any) error {
	event := Event{
		Type:    eventType,
		Payload: payload,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	err = RedisClient.Publish(ctx, "memory_events", data).Err()
	if err != nil {
		log.Printf("❌ Failed to publish event: %v", err)
		return err
	}

	log.Printf("📣 [REDIS] Published event: %s", eventType)
	if webhookNotify != nil {
		if tenantID, ok := payload["tenant_id"].(string); ok && tenantID != "" {
			webhookNotify(tenantID, eventType, payload)
		}
	}
	return nil
}

// SubscribeEvents subscribes to Redis channel until the pubsub channel closes.
func SubscribeEvents() <-chan Event {
	return SubscribeEventsContext(ctx)
}

// SubscribeEventsContext subscribes to Redis and stops when ctx is cancelled.
//
// We synchronously wait for the SUBSCRIBE confirmation via pubsub.Receive
// before returning, so callers can publish immediately without racing the
// subscription setup. go-redis sends the SUBSCRIBE command asynchronously
// otherwise — without this priming the first publish can be lost on slower
// machines (CI, docker-in-docker).
func SubscribeEventsContext(parent context.Context) <-chan Event {
	pubsub := RedisClient.Subscribe(parent, "memory_events")
	ch := make(chan Event, 16)

	if _, err := pubsub.Receive(parent); err != nil {
		log.Printf("❌ Failed to confirm Redis SUBSCRIBE: %v", err)
		_ = pubsub.Close()
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		defer func() { _ = pubsub.Close() }()
		for {
			select {
			case <-parent.Done():
				return
			case msg, ok := <-pubsub.Channel():
				if !ok {
					return
				}
				var evt Event
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
					log.Printf("❌ Failed to unmarshal event: %v", err)
					continue
				}
				select {
				case ch <- evt:
				case <-parent.Done():
					return
				}
			}
		}
	}()

	return ch
}
