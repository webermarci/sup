package bus

import (
	"context"
	"sync"
	"time"

	"github.com/webermarci/sup"
)

// Computed is a reactive value that updates itself based on its dependencies. It implements both Subscribable and Notifyable interfaces.
type Computed[V any] struct {
	*sup.BaseActor
	broadcaster[V]
	value          V
	update         func() V
	deps           []Watcher
	coalesceWindow time.Duration
	mu             sync.RWMutex
}

// NewComputed creates a new Computed with the given name, update function, and dependencies. The update function is called whenever any of the dependencies notify a change, and the result is broadcast to subscribers.
func NewComputed[V any](name string, update func() V, deps ...Watcher) *Computed[V] {
	return &Computed[V]{
		BaseActor:      sup.NewBaseActor(name),
		broadcaster:    broadcaster[V]{buffer: 16},
		value:          update(),
		update:         update,
		deps:           deps,
		coalesceWindow: 5 * time.Millisecond,
	}
}

// WithCoalesceWindow configures the delay used to batch concurrent dependency updates.
// Defaults to 5ms, which is typically enough to catch immediately adjacent graph updates.
func (c *Computed[V]) WithCoalesceWindow(window time.Duration) *Computed[V] {
	c.coalesceWindow = window
	return c
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (c *Computed[V]) WithSubscriberBuffer(buffer int) *Computed[V] {
	c.broadcaster.buffer = buffer
	return c
}

// Read returns the current value of the Computed. It acquires a read lock to ensure thread-safe access to the value.
func (c *Computed[V]) Read() V {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Subscribe returns a channel that receives updates whenever the Computed's value changes. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The current value is sent to the channel immediately upon subscription.
func (c *Computed[V]) Subscribe(ctx context.Context) <-chan V {
	return c.broadcaster.subscribeValues(ctx, c.Read(), true)
}

// Watch returns a channel that receives notifications whenever any of the dependencies of the Computed change. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The channel will receive a notification immediately upon subscription.
func (c *Computed[V]) Watch(ctx context.Context) <-chan struct{} {
	return c.broadcaster.subscribeNotifications(ctx, true)
}

// Run is the main loop for the Computed actor. It subscribes to all dependencies and listens for notifications. Whenever any dependency notifies a change, it calls the update function to compute the new value, updates its internal state, and broadcasts the new value to subscribers. The loop continues until the context is canceled, at which point it cleans up and exits.
func (c *Computed[V]) Run(ctx context.Context) error {
	ping := make(chan struct{}, 1)

	for _, dep := range c.deps {
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
			c.broadcaster.closeAll()
			return nil

		case <-ping:
			if !pending {
				pending = true
				coalesce.Reset(c.coalesceWindow)
				coalesceChan = coalesce.C
			}

		case <-coalesceChan:
			pending = false
			coalesceChan = nil

			value := c.update()

			c.mu.Lock()
			c.value = value
			c.mu.Unlock()

			c.broadcaster.notify(value)
		}
	}
}
