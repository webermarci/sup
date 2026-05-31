package sup

import (
	"context"
	"encoding/json"
)

// CallControl dispatches JSON input to a call inbox and returns the reply.
type CallControl interface {
	Control
	Dispatch(context.Context, json.RawMessage) (any, error)
}

type callControl[T any, R any] struct {
	name        string
	inbox       *CallInbox[T, R]
	inputSchema map[string]any
}

// NewCallControl creates a call control for the given inbox.
func NewCallControl[T any, R any](name string, inbox *CallInbox[T, R]) CallControl {
	return &callControl[T, R]{
		name:        name,
		inbox:       inbox,
		inputSchema: schemaOf[T](),
	}
}

// Name returns the control name.
func (c *callControl[T, R]) Name() string {
	return c.name
}

// Kind returns the control kind.
func (c *callControl[T, R]) Kind() ControlKind {
	return ControlKindCall
}

// InputSchema returns the JSON-compatible schema for the control input.
func (c *callControl[T, R]) InputSchema() map[string]any {
	return c.inputSchema
}

// Dispatch decodes raw JSON input, sends it to the inbox, and returns the reply.
func (c *callControl[T, R]) Dispatch(ctx context.Context, raw json.RawMessage) (any, error) {
	var input T

	if len(raw) == 0 {
		raw = []byte("{}")
	}

	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}

	return c.inbox.Call(ctx, input)
}
