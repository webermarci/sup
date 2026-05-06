package sup

import (
	"context"
	"sync"
	"time"
)

// PolledSignal represents a value that is periodically updated by a function and can be subscribed to for updates.
type PolledSignal[V any] struct {
	*BaseActor
	broadcaster   broadcaster[V]
	value         V
	update        func(context.Context) (V, error)
	interval      time.Duration
	equal         func(a, b V) bool
	initialNotify bool
	mu            sync.RWMutex
}

// NewPolledSignal creates a new PolledSignal with the given name and update function.
func NewPolledSignal[V any](name string, update func(context.Context) (V, error)) *PolledSignal[V] {
	return &PolledSignal[V]{
		BaseActor:   NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		update:      update,
		interval:    time.Second,
	}
}

// WithInitialValue sets the initial value of the Signal before any updates occur.
func (s *PolledSignal[V]) WithInitialValue(initial V) *PolledSignal[V] {
	s.value = initial
	return s
}

// WithInterval sets the interval at which the Signal's update function is called to refresh its value.
func (s *PolledSignal[V]) WithInterval(interval time.Duration) *PolledSignal[V] {
	s.interval = interval
	return s
}

// WithSubscriberBuffer configures the buffer size for subscriber channels to prevent blocking on updates.
func (s *PolledSignal[V]) WithSubscriberBuffer(buffer int) *PolledSignal[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (s *PolledSignal[V]) WithInitialNotify(enabled bool) *PolledSignal[V] {
	s.initialNotify = enabled
	return s
}

// WithEqual configures a custom equality function to determine if the Signal's value has changed.
// If not set, the Signal will use the default equality check (==) to compare old and new values.
// This can be useful for complex types where a simple equality check may not be sufficient.
func (s *PolledSignal[V]) WithEqual(eq func(a, b V) bool) *PolledSignal[V] {
	s.equal = eq
	return s
}

// Read returns the current value of the Signal. It acquires a read lock to ensure thread-safe access to the value.
func (s *PolledSignal[V]) Read() V {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Subscribe allows clients to subscribe to updates of the Signal's value.
// It returns a channel that will receive new values whenever they are updated.
// The subscription will automatically clean up when the provided context is canceled.
func (s *PolledSignal[V]) Subscribe(ctx context.Context) <-chan V {
	s.mu.RLock()
	current := s.value
	s.mu.RUnlock()
	return s.broadcaster.subscribeValues(ctx, current, s.initialNotify)
}

// Watch allows clients to subscribe to notifications whenever the Signal's value is updated,
// without receiving the actual value.
// It returns a channel that will receive a notification (empty struct) whenever the value is updated.
// The subscription will automatically clean up when the provided context is canceled.
func (s *PolledSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	return s.broadcaster.subscribeNotifications(ctx, s.initialNotify)
}

// Run starts the Signal's update loop,
// which periodically calls the update function to refresh the Signal's value and notifies subscribers of any changes.
// The loop continues until the provided context is canceled, at which point it will clean up all subscriber channels.
func (s *PolledSignal[V]) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	if v, err := s.update(ctx); err == nil {
		s.mu.Lock()
		s.value = v
		s.mu.Unlock()
		s.broadcaster.notify(v)
	}

	for {
		select {
		case <-ctx.Done():
			s.broadcaster.closeAll()
			return nil
		case <-ticker.C:
			v, err := s.update(ctx)
			if err != nil {
				continue
			}

			s.mu.RLock()
			current := s.value
			s.mu.RUnlock()

			if s.equal != nil && s.equal(current, v) {
				continue
			}

			s.mu.Lock()
			s.value = v
			s.mu.Unlock()

			s.broadcaster.notify(v)
		}
	}
}
