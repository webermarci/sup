package sup

import (
	"context"
	"sync"
)

// Outbox provides a type-safe asynchronous broadcast mechanism.
// T is the message type emitted to all subscribers.
type Outbox[T any] struct {
	subscribers []func(context.Context, T)
	mu          sync.RWMutex
}

// Subscribe registers a new handler. Handlers should ideally be
// non-blocking (like an Inbox.Cast) to keep the system responsive.
func (o *Outbox[T]) Subscribe(handler func(context.Context, T)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.subscribers = append(o.subscribers, handler)
}

// Emit sends a message to all registered handlers.
// It is the caller's responsibility to ensure that handlers do not block indefinitely,
// as this will affect the responsiveness of the system.
// Handlers should ideally use non-blocking calls or manage their own goroutines if
// they need to perform longer work.
func (o *Outbox[T]) Emit(ctx context.Context, message T) {
	o.mu.RLock()
	subscribers := append([]func(context.Context, T){}, o.subscribers...)
	o.mu.RUnlock()

	for _, handler := range subscribers {
		handler(ctx, message)
	}
}
