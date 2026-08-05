package sup

import "context"

// CastInbox delivers asynchronous messages to an actor.
type CastInbox[T any] struct {
	inbox[T]
}

// NewCastInbox creates an unbuffered cast inbox.
func NewCastInbox[T any]() *CastInbox[T] {
	return &CastInbox[T]{inbox: make(inbox[T])}
}

// Cast sends a message, blocking until an actor receives it or the context is
// canceled.
func (i *CastInbox[T]) Cast(ctx context.Context, message T) error {
	return i.send(ctx, message)
}
