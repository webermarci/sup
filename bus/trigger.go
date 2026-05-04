package bus

import (
	"context"
	"sync"

	"github.com/webermarci/sup"
)

// Trigger represents a value that can be updated by a function and subscribed to for updates.
type Trigger[V any] struct {
	*sup.BaseActor
	broadcaster   broadcaster[V]
	value         V
	update        func(context.Context, V) error
	initialNotify bool
	mu            sync.Mutex
}

// NewTrigger creates a new Trigger with the given name and update function.
func NewTrigger[V any](name string, update func(context.Context, V) error) *Trigger[V] {
	return &Trigger[V]{
		BaseActor:   sup.NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		update:      update,
	}
}

// WithInitialValue sets the initial value of the Trigger before any updates occur.
func (t *Trigger[V]) WithInitialValue(initial V) *Trigger[V] {
	t.value = initial
	return t
}

// WithSubscriberBuffer configures the buffer size for subscriber channels to prevent blocking on updates.
func (t *Trigger[V]) WithSubscriberBuffer(buffer int) *Trigger[V] {
	t.broadcaster.buffer = buffer
	return t
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (t *Trigger[V]) WithInitialNotify(enabled bool) *Trigger[V] {
	t.initialNotify = enabled
	return t
}

// Read returns the current value of the Trigger. It acquires a lock to ensure thread-safe access to the value.
func (t *Trigger[V]) Read() V {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.value
}

// Subscribe returns a channel that receives updates whenever the Trigger's value changes.
// If initialNotify is enabled, the current value is sent to the channel immediately upon subscription.
// It acquires a lock to ensure thread-safe access to the current value when subscribing.
func (t *Trigger[V]) Subscribe(ctx context.Context) <-chan V {
	t.mu.Lock()
	current := t.value
	t.mu.Unlock()
	return t.broadcaster.subscribeValues(ctx, current, t.initialNotify)
}

// Watch allows clients to subscribe to notifications whenever the Trigger's value is updated,
// without receiving the actual value. It returns a channel that will receive a notification (empty struct) whenever the value is updated. The subscription will automatically clean up when the provided context is canceled.
func (t *Trigger[V]) Watch(ctx context.Context) <-chan struct{} {
	return t.broadcaster.subscribeNotifications(ctx, t.initialNotify)
}

// Write updates the Trigger's value by calling the update function with the provided value.
// If the update is successful, it notifies all subscribers of the new value.
// It acquires a lock to ensure thread-safe updates to the value.
func (t *Trigger[V]) Write(ctx context.Context, value V) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.update(ctx, value); err != nil {
		return err
	}

	t.value = value
	t.broadcaster.notify(value)
	return nil
}

// Run starts the Trigger's main loop, which waits for the context to be canceled. When the context is canceled, it cleans up all subscriber channels. This method should be run in a separate goroutine.
func (t *Trigger[V]) Run(ctx context.Context) error {
	<-ctx.Done()
	t.broadcaster.closeAll()
	return nil
}
