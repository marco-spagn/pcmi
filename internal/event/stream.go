package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/marco-spagn/pcmi/internal/log"
)

const (
	streamFieldType = "type"
	streamFieldData = "data"
	defaultBlock    = 2 * time.Second
	defaultBatch    = 10
	maxDeliveries   = 5
)

// StreamPublisher writes events to a Redis stream via XADD.
type StreamPublisher struct {
	client *redis.Client
	stream string
}

// NewStreamPublisher returns a publisher for streamKey (empty → StreamKey).
func NewStreamPublisher(client *redis.Client, streamKey string) *StreamPublisher {
	if streamKey == "" {
		streamKey = StreamKey
	}
	return &StreamPublisher{client: client, stream: streamKey}
}

// Publish marshals the event, XADDs it, and returns the Redis stream entry ID.
func (p *StreamPublisher) Publish(ctx context.Context, eventType string, payload map[string]any) (string, error) {
	if p.client == nil {
		return "", errors.New("redis client is nil")
	}
	evt := Event{Type: eventType, Payload: payload}
	data, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	id, err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]interface{}{
			streamFieldType: eventType,
			streamFieldData: string(data),
		},
	}).Result()
	if err != nil {
		return "", err
	}
	return id, nil
}

// StreamHandler processes one stream message. Return nil to ACK.
type StreamHandler func(ctx context.Context, evt Event, streamID string) error

// StreamConsumer reads a consumer group via XREADGROUP and ACKs successful handlers.
type StreamConsumer struct {
	client   *redis.Client
	stream   string
	group    string
	consumer string
	block    time.Duration
	batch    int64
}

// NewStreamConsumer builds a group consumer (empty names get defaults).
func NewStreamConsumer(client *redis.Client, streamKey, group, consumer string) *StreamConsumer {
	if streamKey == "" {
		streamKey = StreamKey
	}
	if group == "" {
		group = WorkerConsumerGroup
	}
	if consumer == "" {
		consumer = "pcmi-worker"
	}
	return &StreamConsumer{
		client:   client,
		stream:   streamKey,
		group:    group,
		consumer: consumer,
		block:    defaultBlock,
		batch:    defaultBatch,
	}
}

// EnsureGroup creates the consumer group on the stream (idempotent).
func (c *StreamConsumer) EnsureGroup(ctx context.Context) error {
	if c.client == nil {
		return errors.New("redis client is nil")
	}
	err := c.client.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

// Consume runs until ctx is cancelled. Successful handlers are ACKed; errors leave entries pending.
func (c *StreamConsumer) Consume(ctx context.Context, handler StreamHandler) error {
	if c.client == nil {
		return errors.New("redis client is nil")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.stream, ">"},
			Count:    c.batch,
			Block:    c.block,
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if err == redis.Nil {
				continue
			}
			log.Error("stream XREADGROUP failed", "err", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				evt, parseErr := decodeStreamMessage(msg)
				if parseErr != nil {
					log.Error("stream decode failed", "id", msg.ID, "err", parseErr)
					_ = c.ack(ctx, msg.ID)
					continue
				}
				if err := handler(ctx, evt, msg.ID); err != nil {
					log.Warn("stream handler failed, no ACK", "id", msg.ID, "err", err)
					continue
				}
				if ackErr := c.ack(ctx, msg.ID); ackErr != nil {
					log.Error("stream XACK failed", "id", msg.ID, "err", ackErr)
				} else {
					IncStreamAck()
				}
			}
		}
	}
}

func (c *StreamConsumer) ack(ctx context.Context, ids ...string) error {
	// ACK must complete even when the read loop ctx is cancelled (shutdown, test teardown).
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return c.client.XAck(ackCtx, c.stream, c.group, ids...).Err()
}

func decodeStreamMessage(msg redis.XMessage) (Event, error) {
	raw, ok := msg.Values[streamFieldData]
	if !ok {
		return Event{}, fmt.Errorf("missing %q field", streamFieldData)
	}
	var data string
	switch v := raw.(type) {
	case string:
		data = v
	case []byte:
		data = string(v)
	default:
		return Event{}, fmt.Errorf("unexpected data type %T", raw)
	}
	var evt Event
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return Event{}, err
	}
	if evt.Type == "" {
		if t, ok := msg.Values[streamFieldType].(string); ok {
			evt.Type = t
		}
	}
	if evt.Payload == nil {
		evt.Payload = make(map[string]any)
	}
	evt.Payload[PayloadKeyStreamID] = msg.ID
	return evt, nil
}

// streamSubscribe tails pcmi:events for SSE/gRPC fan-out (non-grouped XREAD).
func streamSubscribe(parent context.Context) <-chan Event {
	ch := make(chan Event, 16)
	if RedisClient == nil {
		close(ch)
		return ch
	}
	go func() {
		defer close(ch)
		lastID := "$"
		for {
			select {
			case <-parent.Done():
				return
			default:
			}
			streams, err := RedisClient.XRead(parent, &redis.XReadArgs{
				Streams: []string{StreamKey, lastID},
				Count:   defaultBatch,
				Block:   defaultBlock,
			}).Result()
			if err != nil {
				if errors.Is(err, context.Canceled) || parent.Err() != nil {
					return
				}
				if err != redis.Nil {
					log.Error("stream XREAD failed", "err", err)
					time.Sleep(500 * time.Millisecond)
				}
				continue
			}
			for _, stream := range streams {
				for _, msg := range stream.Messages {
					lastID = msg.ID
					evt, err := decodeStreamMessage(msg)
					if err != nil {
						log.Error("stream subscribe decode failed", "err", err)
						continue
					}
					select {
					case ch <- evt:
					case <-parent.Done():
						return
					}
				}
			}
		}
	}()
	return ch
}
