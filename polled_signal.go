package sup

import (
	"context"
	"time"
)

// PolledSignal represents a value that is periodically updated by a function and can be subscribed to for updates.
type PolledSignal[V any] struct {
	*BaseSignal[V]
	update   func(context.Context) (V, error)
	interval time.Duration
}

// NewPolledSignal creates a new PolledSignal with the given name and update function.
func NewPolledSignal[V any](name string, update func(context.Context) (V, error), interval time.Duration) *PolledSignal[V] {
	return &PolledSignal[V]{
		BaseSignal: NewBaseSignal[V](name),
		update:     update,
		interval:   interval,
	}
}

// Run starts the Signal's update loop,
// which periodically calls the update function to refresh the Signal's value and notifies subscribers of any changes.
// The loop continues until the provided context is canceled, at which point it will clean up all subscriber channels.
func (s *PolledSignal[V]) Run(ctx context.Context) error {
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
				continue
			}
			s.set(v)
		}
	}
}
