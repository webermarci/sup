package sup

import (
	"context"
	"sync/atomic"
)

// CallInbox delivers synchronous requests and replies to callers.
type CallInbox[T any, R any] struct {
	requests chan CallRequest[T, R]
}

// NewCallInbox creates an unbuffered call inbox.
func NewCallInbox[T any, R any]() *CallInbox[T, R] {
	return &CallInbox[T, R]{requests: make(chan CallRequest[T, R])}
}

// Call sends a request and waits for a reply or context cancellation.
func (i *CallInbox[T, R]) Call(ctx context.Context, message T) (R, error) {
	var zero R
	replyTo := make(chan result[R], 1)
	req := CallRequest[T, R]{ctx: ctx, payload: message, replyTo: replyTo, replied: &atomic.Bool{}}
	select {
	case i.requests <- req:
	case <-ctx.Done():
		return zero, ctx.Err()
	}

	select {
	case res := <-replyTo:
		return res.value, res.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Receive returns the read-only request channel.
func (i *CallInbox[T, R]) Receive() <-chan CallRequest[T, R] {
	return i.requests
}
