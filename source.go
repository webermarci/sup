package sup

import (
	"context"
)

// EmitFunc sends a value from a source into a signal.
type EmitFunc[V any] func(context.Context, V) error

// Source produces values for a signal.
type Source[V any] interface {
	Run(ctx context.Context, emit EmitFunc[V]) error
}

// SourceFunc adapts a function into a Source.
type SourceFunc[V any] func(context.Context, EmitFunc[V]) error

// Run calls f with the context and emit function.
func (f SourceFunc[V]) Run(ctx context.Context, emit EmitFunc[V]) error {
	return f(ctx, emit)
}
