package sup

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrCallInboxFull is returned when a non-blocking call cannot be queued.
	ErrCallInboxFull = errors.New("sup: call inbox is full")

	// ErrCallInboxClosed is returned when a call is made after the inbox is closed.
	ErrCallInboxClosed = errors.New("sup: call inbox is closed")
)

// CallInbox queues synchronous requests and delivers replies to callers.
type CallInbox[T any, R any] struct {
	channel chan CallRequest[T, R]
	pool    sync.Pool
	closed  atomic.Bool
}

// NewCallInbox creates a call inbox with the given buffer size.
func NewCallInbox[T any, R any](size int) *CallInbox[T, R] {
	return &CallInbox[T, R]{
		channel: make(chan CallRequest[T, R], size),
		pool: sync.Pool{
			New: func() any {
				return make(chan result[R], 1)
			},
		},
	}
}

// Call sends a request and waits for a reply or context cancellation.
func (i *CallInbox[T, R]) Call(ctx context.Context, message T) (R, error) {
	var zero R
	if i.closed.Load() {
		return zero, ErrCallInboxClosed
	}

	ch := i.pool.Get().(chan result[R])

	select {
	case <-ch:
	default:
	}

	req := CallRequest[T, R]{
		payload: message,
		replyTo: ch,
	}

	select {
	case i.channel <- req:
	case <-ctx.Done():
		i.pool.Put(ch)
		return zero, ctx.Err()
	}

	select {
	case res := <-ch:
		i.pool.Put(ch)
		return res.value, res.err
	case <-ctx.Done():
		i.pool.Put(ch)
		return zero, ctx.Err()
	}
}

// Receive returns the read-only request channel for the actor's internal loop.
func (i *CallInbox[T, R]) Receive() <-chan CallRequest[T, R] {
	return i.channel
}

// Close closes the inbox and prevents future calls.
func (i *CallInbox[T, R]) Close() {
	if i.closed.CompareAndSwap(false, true) {
		close(i.channel)
	}
}

// Closed reports whether the inbox is closed.
func (i *CallInbox[T, R]) Closed() bool {
	return i.closed.Load()
}

// Len returns the number of messages currently in the inbox.
func (i *CallInbox[T, R]) Len() int {
	return len(i.channel)
}

// Cap returns the capacity of the inbox buffer.
func (i *CallInbox[T, R]) Cap() int {
	return cap(i.channel)
}
