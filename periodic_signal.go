package sup

import (
	"context"
	"time"
)

// PeriodicSignal represents a value that is periodically updated by a function and can be subscribed to for updates.
type PeriodicSignal[V any] struct {
	*BaseSignal[V]
	update   func(context.Context) (V, error)
	interval time.Duration
	trigger  chan struct{}
}

// NewPeriodicSignal creates a new PeriodicSignal with the given name and update function.
func NewPeriodicSignal[V any](name string, update func(context.Context) (V, error), interval time.Duration) *PeriodicSignal[V] {
	return &PeriodicSignal[V]{
		BaseSignal: NewBaseSignal[V](name),
		update:     update,
		interval:   interval,
		trigger:    make(chan struct{}, 1),
	}
}

// Run starts the Signal's update loop, which periodically calls the update function
// to refresh the Signal's value and notifies subscribers of any changes.
// The loop continues until the provided context is canceled, at which point it will clean up all subscriber channels.
func (s *PeriodicSignal[V]) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	v, err := s.update(ctx)
	if err != nil {
		return err
	}
	s.set(v)

	for {
		select {
		case <-ctx.Done():
			s.closeAll()
			return nil
		case <-ticker.C:
			v, err := s.update(ctx)
			if err != nil {
				return err
			}
			s.set(v)
		case <-s.trigger:
			v, err := s.update(ctx)
			if err != nil {
				return err
			}
			s.set(v)
		}
	}
}

// Trigger requests an immediate refresh of the signal value.
func (s *PeriodicSignal[V]) Trigger() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Inspect returns the specification.
func (s *PeriodicSignal[V]) Inspect() Spec {
	return Spec{
		Kind:         "periodic_signal",
		Dependencies: []string{},
		Metadata: map[string]string{
			"interval": s.interval.String(),
		},
	}
}
