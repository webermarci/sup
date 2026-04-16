package sup

import "reflect"

type result[R any] struct {
	value R
	err   error
}

// CastRequest wraps a payload for asynchronous calls without expecting a reply.
type CastRequest[T any] struct {
	payload T
}

// Payload returns the request's payload.
func (r *CastRequest[T]) Payload() T {
	return r.payload
}

// CallRequest wraps a payload with a reply channel for synchronous calls.
type CallRequest[T any, R any] struct {
	payload T
	replyTo chan result[R]
}

// Payload returns the request's payload.
func (r *CallRequest[T, R]) Payload() T {
	return r.payload
}

// Reply sends the response back to the caller.
// The actor should call this exactly once per request. 
// After calling Reply, the request object should no longer be accessed.
func (r *CallRequest[T, R]) Reply(value R, err error) {
	r.replyTo <- result[R]{value: value, err: err}
	
	// Return the request object to the pool.
	// We zero out the payload to avoid memory leaks if T contains pointers.
	var zero T
	r.payload = zero
	
	t := reflect.TypeFor[CallRequest[T, R]]()
	if p, ok := requestPools.Load(t); ok {
		p.(*requestPool).Put(r)
	}
}
