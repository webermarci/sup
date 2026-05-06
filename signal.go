package sup

import (
	"context"
)

// ReadableSignal represents a value that can be read and subscribed to for updates.
// It also supports watching for changes.
// ReadableSignal is a sup.Actor,
// so it can be run as a goroutine and can be stopped by canceling its context.
type ReadableSignal[V any] interface {
	Actor
	Reader[V]
	Subscriber[V]
	Watcher
}

// WritableSignal represents a value that can be read, updated, and subscribed to for updates.
// It also supports watching for changes.
// WritableSignal is a sup.Actor,
// so it can be run as a goroutine and can be stopped by canceling its context.
type WritableSignal[V any] interface {
	Actor
	ReadableSignal[V]
	Writer[V]
}

// Reader represents a value that can be read. The Read method returns the current value.
type Reader[V any] interface {
	Read() V
}

// Writer represents a value that can be updated by writing to it.
// The Write method may return an error if the update is rejected.
type Writer[V any] interface {
	Write(context.Context, V) error
}

// Watcher represents a value that can be watched for changes.
// The Watch method returns a channel that will receive a notification whenever the value changes.
// The channel will be closed when the context is canceled.
type Watcher interface {
	Watch(ctx context.Context) <-chan struct{}
}

// Subscriber represents a value that can be subscribed to for updates.
// The Subscribe method returns a channel that will receive the updated value whenever it changes.
// The channel will be closed when the context is canceled.
type Subscriber[V any] interface {
	Subscribe(context.Context) <-chan V
}
