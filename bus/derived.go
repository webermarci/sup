package bus

import (
	"context"
	"sync"

	"github.com/webermarci/sup"
)

// Derived is a reactive value that updates itself based on its dependencies. It implements both Subscribable and Notifyable interfaces.
type Derived[V any] struct {
	*sup.BaseActor
	broadcaster[V]
	value  V
	update func() V
	deps   []Notifyable
	mu     sync.RWMutex
}

// NewDerived creates a new Derived with the given name, update function, and dependencies. The update function is called whenever any of the dependencies notify a change, and the result is broadcast to subscribers.
func NewDerived[V any](name string, update func() V, deps ...Notifyable) *Derived[V] {
	return &Derived[V]{
		BaseActor:   sup.NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		value:       update(),
		update:      update,
		deps:        deps,
	}
}

// Read returns the current value of the Derived. It acquires a read lock to ensure thread-safe access to the value.
func (d *Derived[V]) Read() V {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.value
}

// Subscribe returns a channel that receives updates whenever the Derived's value changes. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The current value is sent to the channel immediately upon subscription.
func (d *Derived[V]) Subscribe(ctx context.Context) <-chan V {
	return d.broadcaster.subscribeValues(ctx, d.Read(), true)
}

// Notify returns a channel that receives notifications whenever any of the dependencies of the Derived change. It subscribes to all dependencies and triggers an update whenever any of them notify a change. The channel will receive a notification immediately upon subscription.
func (d *Derived[V]) Notify(ctx context.Context) <-chan struct{} {
	return d.broadcaster.subscribeNotifications(ctx, true)
}

// Run is the main loop for the Derived actor. It subscribes to all dependencies and listens for notifications. Whenever any dependency notifies a change, it calls the update function to compute the new value, updates its internal state, and broadcasts the new value to subscribers. The loop continues until the context is canceled, at which point it cleans up and exits.
func (d *Derived[V]) Run(ctx context.Context) error {
	ping := make(chan struct{}, 1)

	for _, dep := range d.deps {
		ch := dep.Notify(ctx)
		go func(c <-chan struct{}) {
			for range c {
				select {
				case ping <- struct{}{}:
				default:
				}
			}
		}(ch)
	}

	for {
		select {
		case <-ctx.Done():
			d.broadcaster.closeAll()
			return nil

		case <-ping:
			value := d.update()

			d.mu.Lock()
			d.value = value
			d.mu.Unlock()

			d.broadcaster.notify(value)
		}
	}
}
