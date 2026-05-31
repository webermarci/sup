package sup

import (
	"context"
	"encoding/json"
)

// CastControl dispatches JSON input to a cast inbox.
type CastControl interface {
	Control
	Dispatch(context.Context, json.RawMessage) error
}

type castControl[T any] struct {
	name        string
	inbox       *CastInbox[T]
	inputSchema map[string]any
}

// NewCastControl creates a cast control for the given inbox.
func NewCastControl[T any](name string, inbox *CastInbox[T]) CastControl {
	return &castControl[T]{
		name:        name,
		inbox:       inbox,
		inputSchema: schemaOf[T](),
	}
}

// Name returns the control name.
func (c *castControl[T]) Name() string {
	return c.name
}

// Kind returns the control kind.
func (c *castControl[T]) Kind() ControlKind {
	return ControlKindCast
}

// InputSchema returns the JSON-compatible schema for the control input.
func (c *castControl[T]) InputSchema() map[string]any {
	return c.inputSchema
}

// Dispatch decodes raw JSON input and sends it to the inbox.
func (c *castControl[T]) Dispatch(ctx context.Context, raw json.RawMessage) error {
	var input T

	if len(raw) == 0 {
		raw = []byte("{}")
	}

	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}

	return c.inbox.Cast(ctx, input)
}
