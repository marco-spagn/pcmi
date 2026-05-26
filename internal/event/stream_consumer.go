package event

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/marco-spagn/pcmi/internal/log"
)

const (
	pendingRecoverInterval = 30 * time.Second
	pendingMinIdle         = 60 * time.Second
	pendingClaimBatch      = 10
)

// StartPendingRecovery periodically claims idle pending messages and reprocesses them.
func (c *StreamConsumer) StartPendingRecovery(ctx context.Context, handler StreamHandler) {
	go func() {
		ticker := time.NewTicker(pendingRecoverInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.recoverPending(ctx, handler)
			}
		}
	}()
}

func (c *StreamConsumer) recoverPending(ctx context.Context, handler StreamHandler) {
	if c.client == nil {
		return
	}
	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.stream,
		Group:  c.group,
		Start:  "-",
		End:    "+",
		Count:  pendingClaimBatch,
	}).Result()
	if err != nil {
		log.Warn("stream XPENDING failed", "err", err)
		return
	}
	SetStreamPending(len(pending))
	if len(pending) == 0 {
		return
	}
	ids := make([]string, 0, len(pending))
	deliveryByID := make(map[string]int64, len(pending))
	for _, p := range pending {
		ids = append(ids, p.ID)
		deliveryByID[p.ID] = p.RetryCount
	}
	claimed, err := c.client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   c.stream,
		Group:    c.group,
		Consumer: c.consumer,
		MinIdle:  pendingMinIdle,
		Messages: ids,
	}).Result()
	if err != nil {
		log.Warn("stream XCLAIM failed", "err", err)
		return
	}
	for _, msg := range claimed {
		if deliveryByID[msg.ID] >= maxDeliveries {
			c.moveToDLQ(ctx, msg)
			continue
		}
		evt, parseErr := decodeStreamMessage(msg)
		if parseErr != nil {
			log.Error("pending decode failed", "id", msg.ID, "err", parseErr)
			_ = c.ack(ctx, msg.ID)
			continue
		}
		if err := handler(ctx, evt, msg.ID); err != nil {
			log.Warn("pending handler failed", "id", msg.ID, "err", err)
			continue
		}
		if ackErr := c.ack(ctx, msg.ID); ackErr != nil {
			log.Error("pending XACK failed", "id", msg.ID, "err", ackErr)
		} else {
			IncStreamAck()
		}
	}
}

func (c *StreamConsumer) moveToDLQ(ctx context.Context, msg redis.XMessage) {
	evt, err := decodeStreamMessage(msg)
	values := map[string]interface{}{
		"original_id": msg.ID,
		"reason":      "max_deliveries",
	}
	if err == nil {
		if data, mErr := json.Marshal(evt); mErr == nil {
			values["data"] = string(data)
		}
	}
	_, dlqErr := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: DLQStreamKey,
		Values: values,
	}).Result()
	if dlqErr != nil {
		log.Error("stream DLQ XADD failed", "id", msg.ID, "err", dlqErr)
		return
	}
	IncStreamDLQ()
	if ackErr := c.ack(ctx, msg.ID); ackErr != nil {
		log.Error("stream DLQ XACK failed", "id", msg.ID, "err", ackErr)
	}
}
