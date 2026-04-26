package sup

import (
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

// IsClosed checks if the mailbox has been closed.
func (m *Mailbox) IsClosed() bool {
	return m.closed.Load()
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
