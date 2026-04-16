package sup

import "context"

// enqueue sends value into mb.ch, translating a send-on-closed panic into ErrMailboxClosed.
func enqueue(mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	mb.ch <- value
	return nil
}

// enqueueContext sends value into mb.ch or returns ctx.Err() if context cancels.
func enqueueContext(ctx context.Context, mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	// Fast-path: if we can send immediately, do it (avoids the select overhead).
	select {
	case mb.ch <- value:
		return nil
	default:
	}

	// Contended path: wait either for the mailbox to accept the message or the ctx to cancel.
	select {
	case mb.ch <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// tryEnqueue attempts to send without blocking and returns ErrMailboxFull if the buffer is full.
func tryEnqueue(mb *Mailbox, value any) (err error) {
	if mb.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

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

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	// Fast-path: if we can send immediately, do it (avoids the select overhead).
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
