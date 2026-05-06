package sup

import (
	"context"
	"time"
)

// ComputedSignal is a reactive value that updates itself based on its dependencies.
// It implements both Subscribable and Notifyable interfaces.
type ComputedSignal[V any] struct {
	*BaseSignal[V]
	update         func() V
	deps           []WatcherSignal
	coalesceWindow time.Duration
}

// NewComputedSignal creates a new ComputedSignal with the given name, update function, and dependencies.
// The update function is called whenever any of the dependencies notify a change,
// and the result is broadcast to subscribers.
func NewComputedSignal[V any](name string, update func() V, deps ...WatcherSignal) *ComputedSignal[V] {
	s := &ComputedSignal[V]{
		BaseSignal:     NewBaseSignal[V](name),
		update:         update,
		deps:           deps,
		coalesceWindow: 5 * time.Millisecond,
	}
	s.value = update()

	return s
}

// SetCoalesceWindow configures the duration to wait after receiving a notification from any dependency before triggering an update.
// This allows for coalescing multiple rapid updates into a single update, improving efficiency.
// It acquires a lock to ensure thread-safe access to the coalesceWindow configuration.
func (s *ComputedSignal[V]) SetCoalesceWindow(window time.Duration) {
	s.mu.Lock()
	s.coalesceWindow = window
	s.mu.Unlock()
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
			s.closeAll()
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
			s.set(value)
		}
	}
}
