package sup

import (
	"context"
)

// PushedSignal represents a value that can be updated by a function and subscribed to for updates.
type PushedSignal[V any] struct {
	*BaseSignal[V]
	update func(context.Context, V) error
}

// NewPushedSignal creates a new PushedSignal with the given name and update function.
func NewPushedSignal[V any](name string, update func(context.Context, V) error) *PushedSignal[V] {
	return &PushedSignal[V]{
		BaseSignal: NewBaseSignal[V](name),
		update:     update,
	}
}

// Write updates the PushedSignal's value by calling the update function with the provided value.
// If the update is successful, it notifies all subscribers of the new value.
// It acquires a lock to ensure thread-safe updates to the value.
func (s *PushedSignal[V]) Write(ctx context.Context, value V) error {
	if err := s.update(ctx, value); err != nil {
		return err
	}

	s.set(value)
	return nil
}

// Run starts the PushedSignal's main loop, which waits for the context to be canceled. When the context is canceled, it cleans up all subscriber channels. This method should be run in a separate goroutine.
func (s *PushedSignal[V]) Run(ctx context.Context) error {
	<-ctx.Done()
	s.closeAll()
	return nil
}
