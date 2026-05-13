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
)

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
	return nil
}

// SubscribeEvents subscribes to Redis channel
func SubscribeEvents() <-chan Event {
	pubsub := RedisClient.Subscribe(ctx, "memory_events")
	ch := make(chan Event)

	go func() {
		defer pubsub.Close()
		for msg := range pubsub.Channel() {
			var event Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				log.Printf("❌ Failed to unmarshal event: %v", err)
				continue
			}
			ch <- event
		}
	}()

	return ch
}
