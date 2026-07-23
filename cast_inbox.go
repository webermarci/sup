package sup

import "context"

// CastInbox queues asynchronous messages for an actor.
type CastInbox[T any] struct {
	inbox[T]
}

// NewCastInbox creates a cast inbox with the given buffer size.
func NewCastInbox[T any](size int) *CastInbox[T] {
	return &CastInbox[T]{inbox: make(inbox[T], size)}
}

// Cast sends a message, blocking until it is queued or the context is canceled.
func (i *CastInbox[T]) Cast(ctx context.Context, message T) error {
	return i.send(ctx, message, true)
}

// TryCast sends a message without blocking when the inbox is full.
func (i *CastInbox[T]) TryCast(ctx context.Context, message T) error {
	return i.send(ctx, message, false)
}
