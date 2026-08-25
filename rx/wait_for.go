package rx

import (
	"context"
	"errors"
)

// ErrReadableClosed is returned when a readable subscription closes before
// WaitFor accepts a value while its context is still active.
var ErrReadableClosed = errors.New("rx: readable closed before condition matched")

// WaitFor waits until accept returns true for a value from readable and returns
// that value. The current value is considered because every subscription starts
// with it. WaitFor owns and cancels its subscription before returning.
func WaitFor[V any](
	ctx context.Context,
	readable Readable[V],
	accept func(V) bool,
) (V, error) {
	var zero V
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	values := readable.Subscribe(waitCtx)

	for {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case value, ok := <-values:
			if !ok {
				if err := ctx.Err(); err != nil {
					return zero, err
				}
				return zero, ErrReadableClosed
			}
			if err := ctx.Err(); err != nil {
				return zero, err
			}
			if accept(value) {
				return value, nil
			}
		}
	}
}
