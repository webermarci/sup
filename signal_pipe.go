package sup

import (
	"context"
)

// SignalPipe is an actor that connects a ReadableSignal to a WritableSignal,
// forwarding updates from the source to the destination.
type SignalPipe[V any] struct {
	*BaseActor
	src  ReadableSignal[V]
	dest WritableSignal[V]
}

// NewSignalPipe creates a new SignalPipe connecting a source to a destination.
func NewSignalPipe[V any](name string, src ReadableSignal[V], dest WritableSignal[V]) *SignalPipe[V] {
	return &SignalPipe[V]{
		BaseActor: NewBaseActor(name),
		src:       src,
		dest:      dest,
	}
}

// Run is the main actor loop. It forwards values from the source to the destination.
func (p *SignalPipe[V]) Run(ctx context.Context) error {
	in := p.src.Subscribe(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil

		case v, ok := <-in:
			if !ok {
				return nil
			}

			if err := p.dest.Write(ctx, v); err != nil {
				return err
			}
		}
	}
}
