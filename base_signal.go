package sup

import (
	"context"
	"sync"
)

// BaseSignal centralizes broadcasting/subscription behavior for signals.
// It intentionally does NOT manage the value or its locking; concrete signals
// keep their own value + mutex and call the base's notify/subscribe helpers.
type BaseSignal[V any] struct {
	*BaseActor
	broadcaster   broadcaster[V]
	value         V
	initialNotify bool
	equal         func(a, b V) bool
	mu            sync.RWMutex
}

// NewBaseSignal creates a BaseSignal with sane defaults.
func NewBaseSignal[V any](name string) *BaseSignal[V] {
	return &BaseSignal[V]{
		BaseActor:   NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 32},
	}
}

// SetInitialValue sets the initial value of the Signal before any updates occur. It acquires a lock to ensure thread-safe access to the value.
func (b *BaseSignal[V]) SetInitialValue(v V) {
	b.mu.Lock()
	b.value = v
	b.mu.Unlock()
}

// SetInitialNotify configures whether to notify subscribers immediately with the current value upon subscription.
// It acquires a lock to ensure thread-safe access to the initialNotify flag.
func (b *BaseSignal[V]) SetInitialNotify(enabled bool) {
	b.mu.Lock()
	b.initialNotify = enabled
	b.mu.Unlock()
}

// SetBroadcasterBuffer configures the internal broadcaster buffer size.
// It acquires a lock to ensure thread-safe access to the broadcaster's buffer configuration.
func (b *BaseSignal[V]) SetBroadcasterBuffer(buffer int) {
	b.broadcaster.mu.Lock()
	b.broadcaster.buffer = buffer
	b.broadcaster.mu.Unlock()
}

// SetEqual configures the equality function used to determine if a new value is different from
// the current value before notifying subscribers.
// This can help prevent unnecessary notifications when the value hasn't actually changed.
// It acquires a lock to ensure thread-safe access to the equal function.
func (b *BaseSignal[V]) SetEqual(eq func(a, b V) bool) {
	b.mu.Lock()
	b.equal = eq
	b.mu.Unlock()
}

// Read returns the current stored value (thread-safe).
func (b *BaseSignal[V]) Read() V {
	b.mu.RLock()
	v := b.value
	b.mu.RUnlock()
	return v
}

// Subscribe returns a channel that receives updates whenever the Signal's value changes.
// If initialNotify is enabled, the current value is sent to the channel immediately upon subscription.
// It acquires a read lock to ensure thread-safe access to the current value when subscribing.
func (b *BaseSignal[V]) Subscribe(ctx context.Context) <-chan V {
	b.mu.RLock()
	current := b.value
	initial := b.initialNotify
	b.mu.RUnlock()
	return b.broadcaster.subscribeValues(ctx, current, initial)
}

// Watch allows clients to subscribe to notifications whenever the Signal's value is updated, without receiving the value itself.
// If initialNotify is enabled, a notification is sent to the channel immediately upon subscription.
func (b *BaseSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	b.mu.RLock()
	initial := b.initialNotify
	b.mu.RUnlock()
	return b.broadcaster.subscribeNotifications(ctx, initial)
}

func (b *BaseSignal[V]) set(v V) {
	b.mu.Lock()
	if b.equal != nil && b.equal(b.value, v) {
		b.mu.Unlock()
		return
	}
	b.value = v
	b.mu.Unlock()
	b.broadcaster.notify(v)
}

func (b *BaseSignal[V]) closeAll() {
	b.broadcaster.closeAll()
}
