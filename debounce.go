package sup

import (
	"context"
	"time"
)

type debounceProcessor[V any] struct {
	wait   time.Duration
	latest V
	seq    uint64
}

// Debounce returns a processor that emits the latest value after wait has elapsed.
func Debounce[V any](wait time.Duration) Processor[V] {
	return &debounceProcessor[V]{
		wait: wait,
	}
}

// Process stores the latest value and schedules a debounce timer.
func (p *debounceProcessor[V]) Process(
	ctx context.Context,
	value V,
	runtime ProcessorRuntime[V],
) error {
	p.latest = value
	p.seq++

	runtime.After(p.wait, p.seq)

	return nil
}

// OnTimer emits the latest value when the active debounce timer fires.
func (p *debounceProcessor[V]) OnTimer(
	ctx context.Context,
	timerID uint64,
	runtime ProcessorRuntime[V],
) error {
	if timerID != p.seq {
		return nil
	}

	runtime.Emit(p.latest)

	return nil
}

// Reset clears the pending debounce value and sequence.
func (p *debounceProcessor[V]) Reset() {
	var zero V

	p.latest = zero
	p.seq = 0
}
