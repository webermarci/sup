package sup

import (
	"context"
	"errors"
	"sync/atomic"
)

var (
	// ErrCastInboxFull is returned when a non-blocking cast cannot be queued.
	ErrCastInboxFull = errors.New("sup: cast inbox is full")

	// ErrCastInboxClosed is returned when a cast is made after the inbox is closed.
	ErrCastInboxClosed = errors.New("sup: cast inbox is closed")
)

// CastInbox queues asynchronous messages for an actor.
type CastInbox[T any] struct {
	channel chan T
	closed  atomic.Bool
}

// NewCastInbox creates a cast inbox with the given buffer size.
func NewCastInbox[T any](size int) *CastInbox[T] {
	return &CastInbox[T]{
		channel: make(chan T, size),
	}
}

// Cast sends a message, blocking until it is queued or the context is canceled.
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

// TryCast sends a message without blocking when the inbox is full.
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

// Close closes the inbox and prevents future casts.
func (i *CastInbox[T]) Close() {
	if i.closed.CompareAndSwap(false, true) {
		close(i.channel)
	}
}

// Closed reports whether the inbox is closed.
func (i *CastInbox[T]) Closed() bool {
	return i.closed.Load()
}

// Len returns the number of messages currently in the inbox.
func (i *CastInbox[T]) Len() int {
	return len(i.channel)
}

// Cap returns the capacity of the inbox buffer.
func (i *CastInbox[T]) Cap() int {
	return cap(i.channel)
}
