package sup

import "context"

// Call sends a message to an actor and waits indefinitely for a reply.
func Call[T any, R any](mb *Mailbox, message T) (R, error) {
	var zero R

	pool := getPool[R]()
	replyCh := pool.Get().(chan result[R])

	req := CallRequest[T, R]{
		Message: message,
		replyTo: replyCh,
	}

	if err := mb.Cast(req); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	res := <-replyCh
	putReplyCh(pool, replyCh)
	return res.value, res.err
}

// CallContext sends a message to an actor and waits for a reply until the context expires.
func CallContext[T any, R any](ctx context.Context, mb *Mailbox, message T) (R, error) {
	var zero R

	pool := getPool[R]()
	replyCh := pool.Get().(chan result[R])

	req := CallRequest[T, R]{
		Message: message,
		replyTo: replyCh,
	}

	if err := mb.CastContext(ctx, req); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	select {
	case res := <-replyCh:
		putReplyCh(pool, replyCh)
		return res.value, res.err
	case <-ctx.Done():
		// The actor may still call Reply() after the context expires.
		// Drain the channel in a goroutine so the actor never blocks.
		go func() {
			<-replyCh
			putReplyCh(pool, replyCh)
		}()
		return zero, ctx.Err()
	}
}

// TryCall attempts to enqueue a request without blocking.
// If the enqueue succeeds, it waits indefinitely for the reply.
// Use TryCallContext if an escape hatch is needed while waiting for the reply.
func TryCall[T any, R any](mb *Mailbox, message T) (R, error) {
	var zero R

	pool := getPool[R]()
	replyCh := pool.Get().(chan result[R])

	req := CallRequest[T, R]{
		Message: message,
		replyTo: replyCh,
	}

	if err := mb.TryCast(req); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	res := <-replyCh
	putReplyCh(pool, replyCh)
	return res.value, res.err
}

// TryCallContext attempts to enqueue a request without blocking.
// If the enqueue succeeds, it waits for a reply until the context expires.
func TryCallContext[T any, R any](ctx context.Context, mb *Mailbox, message T) (R, error) {
	var zero R

	pool := getPool[R]()
	replyCh := pool.Get().(chan result[R])

	req := CallRequest[T, R]{
		Message: message,
		replyTo: replyCh,
	}

	if err := mb.TryCast(req); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	select {
	case res := <-replyCh:
		putReplyCh(pool, replyCh)
		return res.value, res.err
	case <-ctx.Done():
		go func() {
			<-replyCh
			putReplyCh(pool, replyCh)
		}()
		return zero, ctx.Err()
	}
}
