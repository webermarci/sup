package sup

import "context"

// CastInbox delivers asynchronous messages to an actor.
type CastInbox[T any] struct {
	messages chan T
}

// NewCastInbox creates an unbuffered cast inbox.
func NewCastInbox[T any]() *CastInbox[T] {
	return &CastInbox[T]{messages: make(chan T)}
}

// Cast sends a message, blocking until an actor receives it or the context is
// canceled.
func (i *CastInbox[T]) Cast(ctx context.Context, message T) error {
	select {
	case i.messages <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Receive returns the read-only message channel.
func (i *CastInbox[T]) Receive() <-chan T {
	return i.messages
}
