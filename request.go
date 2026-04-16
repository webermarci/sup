package sup

type result[R any] struct {
	value R
	err   error
}

// Request wraps a message with a reply channel for synchronous calls.
// replyTo is always set when constructed via Call or TryCall.
type CallRequest[T any, R any] struct {
	Message T
	replyTo chan result[R]
}

// Reply sends the response back to the caller.
// The actor should call this exactly once per request, and must not close the reply channel.
func (r *CallRequest[T, R]) Reply(value R, err error) {
	r.replyTo <- result[R]{value: value, err: err}
}
