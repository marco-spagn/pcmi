package event

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrRedisNotInitialized is returned when RedisClient was never set (InitRedis).
var ErrRedisNotInitialized = errors.New("redis client not initialized")

var (
	RedisClient *redis.Client
	ctx         = context.Background()

	webhookNotify func(tenantID, eventType string, payload map[string]any)
)

// SetWebhookNotifier registers a callback for outbound webhook delivery (best-effort).
func SetWebhookNotifier(fn func(tenantID, eventType string, payload map[string]any)) {
	webhookNotify = fn
}

// InitRedis initializes the Redis connection with exponential-backoff retry.
// BUG-FIX-4: the original code called log.Fatalf on the first Ping failure,
// killing the process for any transient Redis unavailability at startup
// (e.g. K8s rolling deploy where Redis pod is briefly in Terminating state).
// We now retry 6 times (max ~32 s total) before giving up.
func InitRedis(addr string) {
	if RedisClient != nil {
		_ = RedisClient.Close()
	}
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "",
		DB:       0,
	})

	const maxAttempts = 6
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := RedisClient.Ping(ctx).Result()
		if err == nil {
			log.Println("Connected to Redis")
			return
		}
		if attempt == maxAttempts {
			// Only fatal after all retries exhausted.
			log.Fatalf(" Failed to connect to Redis after %d attempts: %v", maxAttempts, err)
		}
		log.Printf("⏳ Redis not ready (attempt %d/%d): %v — retrying in %s",
			attempt, maxAttempts, err, backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > 16*time.Second {
			backoff = 16 * time.Second
		}
	}
}

// PublishEvent publishes an event to Redis (streams by default, pub/sub when EVENT_BACKEND=pubsub).
func PublishEvent(eventType string, payload map[string]any) error {
	if RedisClient == nil {
		return ErrRedisNotInitialized
	}
	if useStreams() {
		return publishStream(eventType, payload)
	}
	return publishPubSub(eventType, payload)
}

func publishStream(eventType string, payload map[string]any) error {
	if payload == nil {
		payload = make(map[string]any)
	}
	if RedisClient == nil {
		return ErrRedisNotInitialized
	}
	pub := NewStreamPublisher(RedisClient, StreamKey)
	streamID, err := pub.Publish(ctx, eventType, payload)
	if err != nil {
		log.Printf("Failed to XADD event: %v", err)
		return err
	}
	log.Printf("[REDIS STREAM] Published event: %s id=%s", eventType, streamID)
	notifyWebhook(eventType, payload)
	return nil
}

func publishPubSub(eventType string, payload map[string]any) error {
	if RedisClient == nil {
		return ErrRedisNotInitialized
	}
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
		log.Printf("Failed to publish event: %v", err)
		return err
	}

	log.Printf("[REDIS] Published event: %s", eventType)
	notifyWebhook(eventType, payload)
	return nil
}

func notifyWebhook(eventType string, payload map[string]any) {
	if webhookNotify != nil {
		if tenantID, ok := payload["tenant_id"].(string); ok && tenantID != "" {
			webhookNotify(tenantID, eventType, payload)
		}
	}
}

// SubscribeEvents subscribes to PCMI events until the channel closes.
func SubscribeEvents() <-chan Event {
	return SubscribeEventsContext(ctx)
}

// SubscribeEventsContext subscribes and stops when ctx is cancelled.
func SubscribeEventsContext(parent context.Context) <-chan Event {
	if useStreams() {
		return streamSubscribe(parent)
	}
	return pubsubSubscribe(parent)
}

func pubsubSubscribe(parent context.Context) <-chan Event {
	ch := make(chan Event, 16)
	if RedisClient == nil {
		close(ch)
		return ch
	}
	pubsub := RedisClient.Subscribe(parent, "memory_events")

	if _, err := pubsub.Receive(parent); err != nil {
		log.Printf("Failed to confirm Redis SUBSCRIBE: %v", err)
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
					log.Printf("Failed to unmarshal event: %v", err)
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

// WorkerConsumerName returns a unique consumer name for this worker process.
func WorkerConsumerName() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return "pcmi-worker-" + host
	}
	return "pcmi-worker"
}

// NewWorkerStreamConsumer builds the default worker group consumer.
func NewWorkerStreamConsumer() *StreamConsumer {
	return NewStreamConsumer(RedisClient, StreamKey, WorkerConsumerGroup, WorkerConsumerName())
}
