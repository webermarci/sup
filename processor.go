package sup

import (
	"context"
	"time"
)

// Resettable represents a processor that can reset its internal state.
type Resettable interface {
	Reset()
}

// Processor transforms, filters, or delays values written to a signal.
type Processor[V any] interface {
	Process(ctx context.Context, value V, runtime ProcessorRuntime[V]) error
	OnTimer(ctx context.Context, timerID uint64, runtime ProcessorRuntime[V]) error
}

// ProcessorRuntime lets processors emit values or schedule timers.
type ProcessorRuntime[V any] interface {
	Emit(V)
	After(delay time.Duration, timerID uint64)
}

// ProcessorFunc adapts a function into a Processor.
type ProcessorFunc[V any] func(context.Context, V, ProcessorRuntime[V]) error

// Process calls f with the incoming value and runtime.
func (f ProcessorFunc[V]) Process(
	ctx context.Context,
	value V,
	runtime ProcessorRuntime[V],
) error {
	return f(ctx, value, runtime)
}

// OnTimer ignores timer callbacks for function processors.
func (f ProcessorFunc[V]) OnTimer(
	ctx context.Context,
	timerID uint64,
	runtime ProcessorRuntime[V],
) error {
	return nil
}
