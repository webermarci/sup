package sup

import (
	"context"
	"time"
)

type throttleProcessor[V any] struct {
	interval    time.Duration
	ready       bool
	pending     V
	hasPending  bool
	timerActive bool
}

// Throttle returns a processor that emits at most one value per interval.
func Throttle[V any](interval time.Duration) Processor[V] {
	return &throttleProcessor[V]{
		interval: interval,
		ready:    true,
	}
}

// Process implements the Processor interface.
func (p *throttleProcessor[V]) Process(
	ctx context.Context,
	value V,
	runtime ProcessorRuntime[V],
) error {
	if p.ready {
		runtime.Emit(value)

		p.ready = false
		p.timerActive = true

		runtime.After(p.interval, 1)

		return nil
	}

	p.pending = value
	p.hasPending = true

	if !p.timerActive {
		p.timerActive = true
		runtime.After(p.interval, 1)
	}

	return nil
}

// OnTimer implements the Processor interface.
func (p *throttleProcessor[V]) OnTimer(
	ctx context.Context,
	timerID uint64,
	runtime ProcessorRuntime[V],
) error {
	if p.hasPending {
		runtime.Emit(p.pending)

		p.hasPending = false
		p.timerActive = true

		runtime.After(p.interval, 1)

		return nil
	}

	p.ready = true
	p.timerActive = false

	return nil
}

// Reset implements the Processor interface.
func (p *throttleProcessor[V]) Reset() {
	var zero V

	p.ready = true
	p.pending = zero
	p.hasPending = false
	p.timerActive = false
}
