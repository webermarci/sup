package sup

import (
	"context"
	"sync"
)

// PushedSignal represents a value that can be updated by a function and subscribed to for updates.
type PushedSignal[V any] struct {
	*BaseActor
	broadcaster   broadcaster[V]
	value         V
	update        func(context.Context, V) error
	initialNotify bool
	mu            sync.Mutex
}

// NewPushedSignal creates a new PushedSignal with the given name and update function.
func NewPushedSignal[V any](name string, update func(context.Context, V) error) *PushedSignal[V] {
	return &PushedSignal[V]{
		BaseActor:   NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		update:      update,
	}
}

// WithInitialValue sets the initial value of the PushedSignal before any updates occur.
func (s *PushedSignal[V]) WithInitialValue(initial V) *PushedSignal[V] {
	s.value = initial
	return s
}

// WithSubscriberBuffer configures the buffer size for subscriber channels to prevent blocking on updates.
func (s *PushedSignal[V]) WithSubscriberBuffer(buffer int) *PushedSignal[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (s *PushedSignal[V]) WithInitialNotify(enabled bool) *PushedSignal[V] {
	s.initialNotify = enabled
	return s
}

// Read returns the current value of the PushedSignal. It acquires a lock to ensure thread-safe access to the value.
func (s *PushedSignal[V]) Read() V {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

// Subscribe returns a channel that receives updates whenever the PushedSignal's value changes.
// If initialNotify is enabled, the current value is sent to the channel immediately upon subscription.
// It acquires a lock to ensure thread-safe access to the current value when subscribing.
func (s *PushedSignal[V]) Subscribe(ctx context.Context) <-chan V {
	s.mu.Lock()
	current := s.value
	s.mu.Unlock()
	return s.broadcaster.subscribeValues(ctx, current, s.initialNotify)
}

// Watch allows clients to subscribe to notifications whenever the PushedSignal's value is updated,
// without receiving the actual value. It returns a channel that will receive a notification (empty struct) whenever the value is updated. The subscription will automatically clean up when the provided context is canceled.
func (s *PushedSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	return s.broadcaster.subscribeNotifications(ctx, s.initialNotify)
}

// Write updates the PushedSignal's value by calling the update function with the provided value.
// If the update is successful, it notifies all subscribers of the new value.
// It acquires a lock to ensure thread-safe updates to the value.
func (s *PushedSignal[V]) Write(ctx context.Context, value V) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.update(ctx, value); err != nil {
		return err
	}

	s.value = value
	s.broadcaster.notify(value)
	return nil
}

// Run starts the PushedSignal's main loop, which waits for the context to be canceled. When the context is canceled, it cleans up all subscriber channels. This method should be run in a separate goroutine.
func (s *PushedSignal[V]) Run(ctx context.Context) error {
	<-ctx.Done()
	s.broadcaster.closeAll()
	return nil
}
