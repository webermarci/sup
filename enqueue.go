package sup

import "context"

// recoverClosed catches panics from sending to a closed channel.
func recoverClosed(err *error) {
	if r := recover(); r != nil {
		*err = ErrMailboxClosed
	}
}

// enqueue sends value into mb.ch, translating a send-on-closed panic into ErrMailboxClosed.
func enqueue(mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer recoverClosed(&err)

	mb.ch <- value
	return nil
}

// enqueueContext sends value into mb.ch or returns ctx.Err() if context cancels.
func enqueueContext(ctx context.Context, mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer recoverClosed(&err)

	// Fast-path: if we can send immediately, do it.
	select {
	case mb.ch <- value:
		return nil
	default:
	}

	// Contended path.
	select {
	case mb.ch <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// tryEnqueue attempts to send without blocking.
func tryEnqueue(mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer recoverClosed(&err)

	select {
	case mb.ch <- value:
		return nil
	default:
		return ErrMailboxFull
	}
}

// tryEnqueueContext tries to send without blocking, but returns ctx.Err() if the context is done.
func tryEnqueueContext(ctx context.Context, mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer recoverClosed(&err)

	// Fast-path.
	select {
	case mb.ch <- value:
		return nil
	default:
	}

	select {
	case mb.ch <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrMailboxFull
	}
}
