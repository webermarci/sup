package bus

import "context"

// Provider represents any reactive actor that emits values.
// Trigger, Signal, Derived, and Debounce all satisfy this interface.
type Provider[V any] interface {
	Read() V
	Subscribe(ctx context.Context) <-chan V
	Watch(ctx context.Context) <-chan struct{}
}
