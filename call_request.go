package sup

import (
	"context"
	"sync/atomic"
)

type result[R any] struct {
	value R
	err   error
}

// CallRequest wraps a payload with a reply channel for synchronous calls.
type CallRequest[T any, R any] struct {
	ctx     context.Context
	payload T
	replyTo chan result[R]
	replied *atomic.Bool
}

// Context returns the caller's context.
func (r CallRequest[T, R]) Context() context.Context {
	return r.ctx
}

// Payload returns the request's payload.
func (r CallRequest[T, R]) Payload() T {
	return r.payload
}

// Reply sends the response back to the caller. It returns false when the caller
// has canceled or a reply was already sent.
func (r CallRequest[T, R]) Reply(value R, err error) bool {
	select {
	case <-r.Context().Done():
		return false
	default:
	}
	if !r.replied.CompareAndSwap(false, true) {
		return false
	}

	select {
	case r.replyTo <- result[R]{value: value, err: err}:
		return true
	case <-r.Context().Done():
		return false
	}
}
