package sup

import "context"

// Filter returns a processor that emits only values accepted by fn.
func Filter[V any](fn func(V) bool) Processor[V] {
	return filterProcessor[V]{fn: fn}
}

type filterProcessor[V any] struct {
	fn func(V) bool
}

// Process emits the incoming value when fn accepts it.
func (p filterProcessor[V]) Process(
	ctx context.Context,
	value V,
	runtime ProcessorRuntime[V],
) error {
	if p.fn(value) {
		runtime.Emit(value)
	}
	return nil
}

// OnTimer ignores timer callbacks for filter processors.
func (p filterProcessor[V]) OnTimer(
	ctx context.Context,
	timerID uint64,
	runtime ProcessorRuntime[V],
) error {
	return nil
}
