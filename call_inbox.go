package sup

import "context"

// CallInbox delivers synchronous requests and replies to callers. T defines
// the request type or family, and R defines the response type or family that
// handlers may return. Each call specifies the concrete response it expects.
type CallInbox[T any, R any] struct {
	requests chan CallRequest[T, R]
}

// NewCallInbox creates an unbuffered call inbox.
func NewCallInbox[T any, R any]() *CallInbox[T, R] {
	return &CallInbox[T, R]{requests: make(chan CallRequest[T, R])}
}

// Call sends a request and waits for a response of type S or context
// cancellation.
func (i *CallInbox[T, R]) Call[S any](ctx context.Context, message T) (S, error) {
	var zero S
	replyTo := make(chan result[S], 1)
	req := CallRequest[T, R]{
		ctx:     ctx,
		payload: message,
		reply:   newCallReply[R, S](ctx, replyTo),
	}
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
