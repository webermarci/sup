package sup

type result[R any] struct {
	value R
	err   error
}

// CastRequest wraps a payload for asynchronous calls without expecting a reply.
type CastRequest[T any] struct {
	payload T
}

func (r *CastRequest[T]) Payload() T {
	return r.payload
}

// CallRequest wraps a payload with a reply channel for synchronous calls.
// replyTo is always set when constructed via Call or TryCall.
type CallRequest[T any, R any] struct {
	payload T
	replyTo chan result[R]
}

func (r *CallRequest[T, R]) Payload() T {
	return r.payload
}

// Reply sends the response back to the caller.
// The actor should call this exactly once per request, and must not close the reply channel.
func (r *CallRequest[T, R]) Reply(value R, err error) {
	r.replyTo <- result[R]{value: value, err: err}
}
