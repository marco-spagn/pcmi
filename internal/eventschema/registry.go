package eventschema

import (
	"fmt"
	"strings"
)

// Field describes a required or optional payload field.
type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// Schema documents a registered event type.
type Schema struct {
	EventType   string  `json:"event_type"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
	Strict      bool    `json:"strict"`
}

var registry = map[string]Schema{
	"memory.stored": {
		EventType:   "memory.stored",
		Description: "A new memory version was stored",
		Strict:      true,
		Fields: []Field{
			{Name: "id", Type: "number", Required: true, Description: "Memory entry id"},
			{Name: "tenant_id", Type: "string", Required: true},
			{Name: "path", Type: "string", Required: true},
			{Name: "version", Type: "number", Required: true},
		},
	},
	"memory.updated": {
		EventType:   "memory.updated",
		Description: "An existing path was superseded by a new version",
		Strict:      true,
		Fields: []Field{
			{Name: "id", Type: "number", Required: true},
			{Name: "tenant_id", Type: "string", Required: true},
			{Name: "path", Type: "string", Required: true},
			{Name: "version", Type: "number", Required: true},
			{Name: "superseded_id", Type: "number", Required: true},
		},
	},
	"knowledge.distilled": {
		EventType:   "knowledge.distilled",
		Description: "Distilled knowledge was produced for a path prefix",
		Strict:      true,
		Fields: []Field{
			{Name: "tenant_id", Type: "string", Required: true},
			{Name: "path", Type: "string", Required: true},
			{Name: "distilled_id", Type: "number", Required: false},
		},
	},
	"agent.step.completed": {
		EventType:   "agent.step.completed",
		Description: "An agent completed a workflow step",
		Strict:      true,
		Fields: []Field{
			{Name: "step", Type: "string", Required: true},
		},
	},
	"agent.step.failed": {
		EventType:   "agent.step.failed",
		Description: "An agent step failed",
		Strict:      true,
		Fields: []Field{
			{Name: "step", Type: "string", Required: true},
			{Name: "error", Type: "string", Required: true},
		},
	},
	"tool.call.executed": {
		EventType:   "tool.call.executed",
		Description: "A tool invocation finished",
		Strict:      true,
		Fields: []Field{
			{Name: "tool", Type: "string", Required: true},
		},
	},
	"workflow.finished": {
		EventType:   "workflow.finished",
		Description: "A multi-step workflow completed",
		Strict:      false,
		Fields: []Field{
			{Name: "workflow_id", Type: "string", Required: false},
		},
	},
	"reasoning.generated": {
		EventType:   "reasoning.generated",
		Description: "Intermediate reasoning was produced",
		Strict:      false,
		Fields: []Field{
			{Name: "summary", Type: "string", Required: false},
		},
	},
	"contradiction.detected": {
		EventType:   "contradiction.detected",
		Description: "A contradiction between two memories was detected",
		Strict:      true,
		Fields: []Field{
			{Name: "tenant_id", Type: "string", Required: true},
			{Name: "from_path", Type: "string", Required: true},
			{Name: "to_path", Type: "string", Required: true},
			{Name: "confidence", Type: "number", Required: true},
		},
	},
}

// List returns all registered schemas sorted by event_type.
func List() []Schema {
	out := make([]Schema, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	return out
}

// Lookup returns a schema and whether it is registered.
func Lookup(eventType string) (Schema, bool) {
	s, ok := registry[strings.TrimSpace(eventType)]
	return s, ok
}

// Validate checks payload against a registered schema when strict validation applies.
// Unknown event types are allowed (forward compatible).
func Validate(eventType string, payload map[string]interface{}) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("event_type is required")
	}
	schema, ok := registry[eventType]
	if !ok || !schema.Strict {
		return nil
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	var missing []string
	for _, f := range schema.Fields {
		if !f.Required {
			continue
		}
		if _, exists := payload[f.Name]; !exists {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("payload missing required fields for %s: %s", eventType, strings.Join(missing, ", "))
	}
	return nil
}
