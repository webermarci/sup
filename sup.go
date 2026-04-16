package sup

import (
	"context"
)

// Cast sends an asynchronous typed envelope, waiting until it can be enqueued or the mailbox is closed.
// It returns ErrMailboxClosed if the mailbox is closed.
func Cast[T any](mb *Mailbox, payload T) error {
	return enqueue(mb, CastRequest[T]{payload: payload})
}

// CastContext sends an asynchronous typed envelope with context for enqueue cancellation.
// It returns ErrMailboxClosed if the mailbox is closed, or ctx.Err() if the context expires before the message is enqueued.
func CastContext[T any](ctx context.Context, mb *Mailbox, payload T) error {
	return enqueueContext(ctx, mb, CastRequest[T]{payload: payload})
}

// TryCast attempts to send an envelope without blocking.
// It returns ErrMailboxClosed if the mailbox is closed, or ErrMailboxFull immediately if the mailbox buffer is full.
func TryCast[T any](mb *Mailbox, payload T) error {
	return tryEnqueue(mb, CastRequest[T]{payload: payload})
}

// TryCastContext attempts to send an envelope without blocking, but returns ctx.Err() if ctx is done.
// It returns ErrMailboxClosed if the mailbox is closed, or ErrMailboxFull immediately if the mailbox buffer is full.
func TryCastContext[T any](ctx context.Context, mb *Mailbox, payload T) error {
	return tryEnqueueContext(ctx, mb, CastRequest[T]{payload: payload})
}

// Call sends a message to an actor and waits indefinitely for a reply.
func Call[T any, R any](mb *Mailbox, payload T) (R, error) {
	var zero R
	rPool := getReplyPool[R]()
	replyCh := rPool.Get().(chan result[R])

	reqPool := getCallRequestPool[T, R]()
	req := reqPool.Get().(*CallRequest[T, R])
	req.payload = payload
	req.replyTo = replyCh

	if err := enqueue(mb, req); err != nil {
		rPool.Put(replyCh)
		reqPool.Put(req)
		return zero, err
	}

	res := <-replyCh
	rPool.Put(replyCh)
	return res.value, res.err
}

// CallContext sends a message to an actor and waits for a reply until the context expires.
func CallContext[T any, R any](ctx context.Context, mb *Mailbox, payload T) (R, error) {
	var zero R
	replayPool := getReplyPool[R]()
	replyCh := replayPool.Get().(chan result[R])

	reqPool := getCallRequestPool[T, R]()
	req := reqPool.Get().(*CallRequest[T, R])
	req.payload = payload
	req.replyTo = replyCh

	if err := enqueueContext(ctx, mb, req); err != nil {
		replayPool.Put(replyCh)
		reqPool.Put(req)
		return zero, err
	}

	select {
	case res := <-replyCh:
		replayPool.Put(replyCh)
		return res.value, res.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// TryCall attempts to enqueue a request without blocking.
func TryCall[T any, R any](mb *Mailbox, payload T) (R, error) {
	var zero R
	replyPool := getReplyPool[R]()
	replyCh := replyPool.Get().(chan result[R])

	reqPool := getCallRequestPool[T, R]()
	req := reqPool.Get().(*CallRequest[T, R])
	req.payload = payload
	req.replyTo = replyCh

	if err := tryEnqueue(mb, req); err != nil {
		replyPool.Put(replyCh)
		reqPool.Put(req)
		return zero, err
	}

	res := <-replyCh
	replyPool.Put(replyCh)
	return res.value, res.err
}

// TryCallContext attempts to enqueue a request without blocking and waits for reply until ctx expires.
func TryCallContext[T any, R any](ctx context.Context, mb *Mailbox, payload T) (R, error) {
	var zero R
	replyPool := getReplyPool[R]()
	replyCh := replyPool.Get().(chan result[R])

	reqPool := getCallRequestPool[T, R]()
	req := reqPool.Get().(*CallRequest[T, R])
	req.payload = payload
	req.replyTo = replyCh

	if err := tryEnqueueContext(ctx, mb, req); err != nil {
		replyPool.Put(replyCh)
		reqPool.Put(req)
		return zero, err
	}

	select {
	case res := <-replyCh:
		replyPool.Put(replyCh)
		return res.value, res.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}
