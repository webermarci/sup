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
	equal          func(a, b V) bool
	coalesceWindow time.Duration
	initialNotify  bool
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

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (c *Computed[V]) WithInitialNotify(enabled bool) *Computed[V] {
	c.initialNotify = enabled
	return c
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (c *Computed[V]) WithSubscriberBuffer(buffer int) *Computed[V] {
	c.broadcaster.buffer = buffer
	return c
}

// WithEqual configures a custom equality function to determine if the computed value has changed. If not set, the Computed will use the default equality check (==) to compare old and new values. This can be useful for complex types where a simple equality check may not be sufficient.
func (c *Computed[V]) WithEqual(eq func(a, b V) bool) *Computed[V] {
	c.equal = eq
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
	c.mu.RLock()
	current := c.value
	c.mu.RUnlock()
	return c.broadcaster.subscribeValues(ctx, current, c.initialNotify)
}

// Watch returns a channel that receives notifications whenever any of the dependencies of the Computed change. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The channel will receive a notification immediately upon subscription.
func (c *Computed[V]) Watch(ctx context.Context) <-chan struct{} {
	return c.broadcaster.subscribeNotifications(ctx, c.initialNotify)
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
			if c.equal != nil && c.equal(c.value, value) {
				c.mu.Unlock()
				continue
			}
			c.value = value
			c.mu.Unlock()

			c.broadcaster.notify(value)
		}
	}
}
