package sup

import (
	"context"
	"sync"
	"time"
)

// ComputedSignal is a reactive value that updates itself based on its dependencies.
// It implements both Subscribable and Notifyable interfaces.
type ComputedSignal[V any] struct {
	*BaseActor
	broadcaster[V]
	value          V
	update         func() V
	deps           []WatcherSignal
	equal          func(a, b V) bool
	coalesceWindow time.Duration
	initialNotify  bool
	mu             sync.RWMutex
}

// NewComputedSignal creates a new ComputedSignal with the given name, update function, and dependencies.
// The update function is called whenever any of the dependencies notify a change,
// and the result is broadcast to subscribers.
func NewComputedSignal[V any](name string, update func() V, deps ...WatcherSignal) *ComputedSignal[V] {
	return &ComputedSignal[V]{
		BaseActor:      NewBaseActor(name),
		broadcaster:    broadcaster[V]{buffer: 16},
		value:          update(),
		update:         update,
		deps:           deps,
		coalesceWindow: 5 * time.Millisecond,
	}
}

// WithCoalesceWindow configures the delay used to batch concurrent dependency updates.
// Defaults to 5ms, which is typically enough to catch immediately adjacent graph updates.
func (s *ComputedSignal[V]) WithCoalesceWindow(window time.Duration) *ComputedSignal[V] {
	s.coalesceWindow = window
	return s
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (s *ComputedSignal[V]) WithInitialNotify(enabled bool) *ComputedSignal[V] {
	s.initialNotify = enabled
	return s
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (s *ComputedSignal[V]) WithSubscriberBuffer(buffer int) *ComputedSignal[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithEqual configures a custom equality function to determine if the computed value has changed. If not set, the Computed will use the default equality check (==) to compare old and new values. This can be useful for complex types where a simple equality check may not be sufficient.
func (s *ComputedSignal[V]) WithEqual(eq func(a, b V) bool) *ComputedSignal[V] {
	s.equal = eq
	return s
}

// Read returns the current value of the Computed. It acquires a read lock to ensure thread-safe access to the value.
func (s *ComputedSignal[V]) Read() V {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Subscribe returns a channel that receives updates whenever the Computed's value changes. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The current value is sent to the channel immediately upon subscription.
func (s *ComputedSignal[V]) Subscribe(ctx context.Context) <-chan V {
	s.mu.RLock()
	current := s.value
	s.mu.RUnlock()
	return s.broadcaster.subscribeValues(ctx, current, s.initialNotify)
}

// Watch returns a channel that receives notifications whenever any of the dependencies of the Computed change. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The channel will receive a notification immediately upon subscription.
func (s *ComputedSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	return s.broadcaster.subscribeNotifications(ctx, s.initialNotify)
}

// Run is the main loop for the Computed actor. It subscribes to all dependencies and listens for notifications. Whenever any dependency notifies a change, it calls the update function to compute the new value, updates its internal state, and broadcasts the new value to subscribers. The loop continues until the context is canceled, at which point it cleans up and exits.
func (s *ComputedSignal[V]) Run(ctx context.Context) error {
	ping := make(chan struct{}, 1)

	for _, dep := range s.deps {
		ch := dep.Watch(ctx)
		go func(c <-chan struct{}) {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-c:
					if !ok {
						return
					}
					select {
					case ping <- struct{}{}:
					default:
					}
				}
			}
		}(ch)
	}

	coalesce := time.NewTimer(time.Hour)
	if !coalesce.Stop() {
		select {
		case <-coalesce.C:
		default:
		}
	}
	var coalesceChan <-chan time.Time
	pending := false

	for {
		select {
		case <-ctx.Done():
			s.broadcaster.closeAll()
			return nil

		case <-ping:
			if !pending {
				pending = true
				coalesce.Reset(s.coalesceWindow)
				coalesceChan = coalesce.C
			}

		case <-coalesceChan:
			pending = false
			coalesceChan = nil

			value := s.update()

			s.mu.Lock()
			if s.equal != nil && s.equal(s.value, value) {
				s.mu.Unlock()
				continue
			}
			s.value = value
			s.mu.Unlock()

			s.broadcaster.notify(value)
		}
	}
}
