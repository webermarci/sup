package sup

import (
	"context"
	"errors"
	"sync/atomic"
)

var (
	ErrCastInboxFull   = errors.New("sup: cast inbox is full")
	ErrCastInboxClosed = errors.New("sup: cast inbox is closed")
)

// CastInbox is a type-safe, write-only entry point for fire-and-forget messages.
type CastInbox[T any] struct {
	channel chan T
	closed  atomic.Bool
}

// NewCastInbox creates a new inbox for a specific message type.
func NewCastInbox[T any](size int) *CastInbox[T] {
	return &CastInbox[T]{
		channel: make(chan T, size),
	}
}

// Cast pushes a message into the inbox with context for cancellation.
// It blocks if the inbox is full, but returns ctx.Err() if the context expires before the message is enqueued.
func (i *CastInbox[T]) Cast(ctx context.Context, message T) error {
	if i.closed.Load() {
		return ErrCastInboxClosed
	}

	select {
	case i.channel <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryCastContext attempts to push a message without blocking, but returns ctx.Err() if ctx is done.
func (i *CastInbox[T]) TryCast(ctx context.Context, message T) error {
	if i.closed.Load() {
		return ErrCastInboxClosed
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case i.channel <- message:
		return nil
	default:
		return ErrCastInboxFull
	}
}

// Receive returns the read-only channel for the actor's internal loop.
func (i *CastInbox[T]) Receive() <-chan T {
	return i.channel
}

// Close safely shuts down the inbox.
func (i *CastInbox[T]) Close() {
	if i.closed.CompareAndSwap(false, true) {
		close(i.channel)
	}
}

// Len returns the number of messages currently in the inbox.
func (i *CastInbox[T]) Len() int {
	return len(i.channel)
}

// Cap returns the capacity of the inbox buffer.
func (i *CastInbox[T]) Cap() int {
	return cap(i.channel)
}
