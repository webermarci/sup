package sup

import (
	"context"
	"errors"
)

// ErrInboxFull is returned when a non-blocking send cannot be queued.
var ErrInboxFull = errors.New("sup: inbox is full")

type inbox[T any] chan T

func (i inbox[T]) send(ctx context.Context, value T, block bool) error {
	if ctx == nil {
		panic("sup: context cannot be nil")
	}

	if block {
		select {
		case i <- value:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case i <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrInboxFull
	}
}

// Receive returns the read-only queue channel.
func (i inbox[T]) Receive() <-chan T {
	return i
}
