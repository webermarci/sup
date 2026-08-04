package sup

import "context"

type inbox[T any] chan T

func (i inbox[T]) send(ctx context.Context, value T) error {
	if ctx == nil {
		panic("sup: context cannot be nil")
	}

	select {
	case i <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Receive returns the read-only queue channel.
func (i inbox[T]) Receive() <-chan T {
	return i
}
