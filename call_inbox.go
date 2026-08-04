package sup

import (
	"context"
	"sync/atomic"
)

// CallInbox queues synchronous requests and delivers replies to callers.
type CallInbox[T any, R any] struct {
	inbox[CallRequest[T, R]]
}

// NewCallInbox creates a call inbox with the given buffer size.
func NewCallInbox[T any, R any](size int) *CallInbox[T, R] {
	return &CallInbox[T, R]{inbox: make(inbox[CallRequest[T, R]], size)}
}

// Call sends a request and waits for a reply or context cancellation.
func (i *CallInbox[T, R]) Call(ctx context.Context, message T) (R, error) {
	return i.call(ctx, message)
}

func (i *CallInbox[T, R]) call(ctx context.Context, message T) (R, error) {
	var zero R
	replyTo := make(chan result[R], 1)
	req := CallRequest[T, R]{ctx: ctx, payload: message, replyTo: replyTo, replied: &atomic.Bool{}}
	if err := i.send(ctx, req); err != nil {
		return zero, err
	}

	select {
	case res := <-replyTo:
		return res.value, res.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}
