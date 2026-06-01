package sup

import (
	"context"
	"errors"
	"sync"
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
	closed  bool
	mu      sync.RWMutex
}

// NewCallInbox creates a call inbox with the given buffer size.
func NewCallInbox[T any, R any](size int) *CallInbox[T, R] {
	return &CallInbox[T, R]{
		channel: make(chan CallRequest[T, R], size),
	}
}

// Call sends a request and waits for a reply or context cancellation.
func (i *CallInbox[T, R]) Call(ctx context.Context, message T) (R, error) {
	var zero R

	replyTo := make(chan result[R], 1)

	req := CallRequest[T, R]{
		payload: message,
		replyTo: replyTo,
	}

	if err := i.send(ctx, req, true); err != nil {
		return zero, err
	}

	select {
	case res := <-replyTo:
		return res.value, res.err

	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// TryCall attempts to queue a request without blocking on inbox capacity.
// If the request is queued, it still waits for a reply or context cancellation.
func (i *CallInbox[T, R]) TryCall(ctx context.Context, message T) (R, error) {
	var zero R

	replyTo := make(chan result[R], 1)

	req := CallRequest[T, R]{
		payload: message,
		replyTo: replyTo,
	}

	if err := i.send(ctx, req, false); err != nil {
		return zero, err
	}

	select {
	case res := <-replyTo:
		return res.value, res.err

	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

func (i *CallInbox[T, R]) send(ctx context.Context, req CallRequest[T, R], blocking bool) error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.closed {
		return ErrCallInboxClosed
	}

	if blocking {
		select {
		case i.channel <- req:
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	select {
	case i.channel <- req:
		return nil

	case <-ctx.Done():
		return ctx.Err()

	default:
		return ErrCallInboxFull
	}
}

// Receive returns the read-only request channel for the actor's internal loop.
func (i *CallInbox[T, R]) Receive() <-chan CallRequest[T, R] {
	return i.channel
}

// Close closes the inbox and prevents future calls.
//
// Already queued requests may still be received by the actor loop before the
// Receive channel is drained and closed.
func (i *CallInbox[T, R]) Close() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.closed {
		return
	}

	i.closed = true
	close(i.channel)
}

// Closed reports whether the inbox is closed.
func (i *CallInbox[T, R]) Closed() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.closed
}

// Len returns the number of messages currently in the inbox.
func (i *CallInbox[T, R]) Len() int {
	return len(i.channel)
}

// Cap returns the capacity of the inbox buffer.
func (i *CallInbox[T, R]) Cap() int {
	return cap(i.channel)
}
