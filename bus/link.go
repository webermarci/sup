package bus

import (
	"context"

	"github.com/webermarci/sup"
)

// Link is a supervised actor that subscribes to a Provider and writes every
// value it receives to a Writer. It eliminates the need for manual,
// unsupervised goroutines when connecting reactive pipelines.
type Link[V any] struct {
	*sup.BaseActor
	src  Provider[V]
	dest Writer[V]
}

// NewLink creates a new Link connecting a source to a destination.
func NewLink[V any](name string, src Provider[V], dest Writer[V]) *Link[V] {
	return &Link[V]{
		BaseActor: sup.NewBaseActor(name),
		src:       src,
		dest:      dest,
	}
}

// Run is the main actor loop. It forwards values from the source to the destination.
func (l *Link[V]) Run(ctx context.Context) error {
	in := l.src.Subscribe(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil

		case v, ok := <-in:
			if !ok {
				return nil
			}

			_ = l.dest.Write(v)
		}
	}
}
