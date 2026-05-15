package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

type EventsHandler struct{}

func NewEventsHandler() *EventsHandler {
	return &EventsHandler{}
}

func parseEventTypes(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

func eventAllowed(evt event.Event, allowed map[string]struct{}, tenantID string) bool {
	if allowed != nil {
		if _, ok := allowed[evt.Type]; !ok {
			return false
		}
	}
	if tenantID == "" {
		return true
	}
	if evtTenant, ok := evt.Payload["tenant_id"].(string); ok && evtTenant != "" {
		return evtTenant == tenantID
	}
	return true
}

// Stream exposes Redis-backed PCMI events as Server-Sent Events (GET /v1/events).
func (h *EventsHandler) Stream(c *fiber.Ctx) error {
	tenantID, _ := c.Locals(middleware.TenantContextKey).(string)
	allowed := parseEventTypes(c.Query("types"))

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	streamCtx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c.Context().Done()
		cancel()
	}()
	events := event.SubscribeEventsContext(streamCtx)

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-streamCtx.Done():
				return
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": heartbeat %d\n\n", time.Now().Unix()); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case evt, ok := <-events:
				if !ok {
					return
				}
				if !eventAllowed(evt, allowed, tenantID) {
					continue
				}
				data, err := json.Marshal(evt)
				if err != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
}
