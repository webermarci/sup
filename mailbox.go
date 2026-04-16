package sup

import (
	"context"
	"errors"
	"sync/atomic"
)

var (
	// ErrMailboxFull is returned by TryCast when the mailbox buffer is full.
	ErrMailboxFull = errors.New("mailbox is full")
	// ErrMailboxClosed is returned when trying to send to a closed mailbox.
	ErrMailboxClosed = errors.New("mailbox is closed")
)

// Mailbox is a thread-safe message queue for actors.
type Mailbox struct {
	ch     chan any
	closed atomic.Bool
}

// NewMailbox creates a new mailbox with the specified buffer size.
// A size of 0 means unbuffered.
func NewMailbox(size int) *Mailbox {
	return &Mailbox{
		ch: make(chan any, size),
	}
}

// Len returns the current number of messages in the mailbox buffer.
func (m *Mailbox) Len() int {
	return len(m.ch)
}

// Cap returns the total capacity of the mailbox buffer.
func (m *Mailbox) Cap() int {
	return cap(m.ch)
}

// IsClosed checks if the mailbox has been closed.
func (m *Mailbox) IsClosed() bool {
	return m.closed.Load()
}

// Cast sends a message, waiting until it can be enqueued or the mailbox is closed.
// It returns ErrMailboxClosed if the mailbox is closed.
func (m *Mailbox) Cast(message any) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	m.ch <- message
	return nil
}

// CastContext sends a message with context support for cancellation and timeouts.
// It returns ErrMailboxClosed if the mailbox is closed, or ctx.Err() if the context expires before the message is enqueued.
// The context is only used to detect cancellation while trying to enqueue, not for the entire duration of the send operation.
func (m *Mailbox) CastContext(ctx context.Context, message any) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryCast attempts to send a message without blocking.
// It returns ErrMailboxFull immediately if the mailbox buffer is full.
func (m *Mailbox) TryCast(message any) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- message:
		return nil
	default:
		return ErrMailboxFull
	}
}

// TryCastContext attempts to send a message without blocking.
// It returns ErrMailboxFull immediately if the mailbox buffer is full.
// The context is only used to detect cancellation while trying to enqueue, not for the entire duration of the send operation.
func (m *Mailbox) TryCastContext(ctx context.Context, message any) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrMailboxFull
	}
}

// Receive returns the read-only channel to consume messages.
func (m *Mailbox) Receive() <-chan any {
	return m.ch
}

// Close safely closes the mailbox. Subsequent sends fail.
func (m *Mailbox) Close() {
	if m.closed.CompareAndSwap(false, true) {
		close(m.ch)
	}
}
