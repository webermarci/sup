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
	batchingWindow time.Duration
}

// NewComputedSignal creates a new ComputedSignal with the given id, update function, and dependencies.
// The update function is called whenever any of the dependencies notify a change,
// and the result is broadcast to subscribers.
func NewComputedSignal[V any](id string, update func() V, deps ...WatcherSignal) *ComputedSignal[V] {
	s := &ComputedSignal[V]{
		BaseSignal:     NewBaseSignal[V](id),
		update:         update,
		deps:           deps,
		batchingWindow: 5 * time.Millisecond,
	}
	s.value = update()

	return s
}

// SetBatchingWindow configures the duration to wait after receiving a notification from any dependency before triggering an update.
// This allows for coalescing multiple rapid updates into a single update, improving efficiency.
// It acquires a lock to ensure thread-safe access to the batchingWindow configuration.
func (s *ComputedSignal[V]) SetBatchingWindow(window time.Duration) {
	s.mu.Lock()
	s.batchingWindow = window
	s.mu.Unlock()
}

// Run is the main loop for the Computed actor.
// It subscribes to all dependencies and listens for notifications.
// Whenever any dependency notifies a change, it calls the update function to compute the new value,
// updates its internal state, and broadcasts the new value to subscribers.
// The loop continues until the context is canceled, at which point it cleans up and exits.
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
				coalesce.Reset(s.batchingWindow)
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

// Inspect returns the specification.
func (s *ComputedSignal[V]) Inspect() Spec {
	var dependencies []string
	for _, d := range s.deps {
		dependencies = append(dependencies, d.ID())
	}

	return Spec{
		Kind:         "computed_signal",
		Dependencies: dependencies,
		Metadata: map[string]string{
			"batching_window": s.batchingWindow.String(),
		},
	}
}
