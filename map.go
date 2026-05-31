package sup

import "context"

// Map returns a processor that transforms each value with fn.
func Map[V any](fn func(V) V) Processor[V] {
	return mapProcessor[V]{fn: fn}
}

type mapProcessor[V any] struct {
	fn func(V) V
}

// Process transforms the incoming value and emits the result.
func (p mapProcessor[V]) Process(
	ctx context.Context,
	value V,
	runtime ProcessorRuntime[V],
) error {
	runtime.Emit(p.fn(value))
	return nil
}

// OnTimer ignores timer callbacks for map processors.
func (p mapProcessor[V]) OnTimer(
	ctx context.Context,
	timerID uint64,
	runtime ProcessorRuntime[V],
) error {
	return nil
}
