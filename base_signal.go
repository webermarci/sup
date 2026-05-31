package sup

import (
	"context"
	"sync"
)

type baseSignal[V any] struct {
	*BaseActor
	broadcaster   broadcaster[V]
	value         V
	initialNotify bool
	equal         func(a, b V) bool
	mu            sync.RWMutex
}

func newBaseSignal[V any](id string, initial V) *baseSignal[V] {
	return &baseSignal[V]{
		BaseActor:   NewBaseActor(id),
		broadcaster: broadcaster[V]{buffer: 32},
		value:       initial,
	}
}

func (b *baseSignal[V]) setInitialNotify(enabled bool) {
	b.mu.Lock()
	b.initialNotify = enabled
	b.mu.Unlock()
}

func (b *baseSignal[V]) setBroadcasterBuffer(buffer int) {
	b.broadcaster.mu.Lock()
	b.broadcaster.buffer = buffer
	b.broadcaster.mu.Unlock()
}

func (b *baseSignal[V]) setEqual(eq func(a, b V) bool) {
	b.mu.Lock()
	b.equal = eq
	b.mu.Unlock()
}

func (b *baseSignal[V]) set(v V) {
	b.mu.Lock()
	if b.equal != nil && b.equal(b.value, v) {
		b.mu.Unlock()
		return
	}
	b.value = v
	b.mu.Unlock()
	b.broadcaster.notify(v)
}

func (b *baseSignal[V]) setLocked(v V) bool {
	if b.equal != nil && b.equal(b.value, v) {
		return false
	}
	b.value = v
	return true
}

func (b *baseSignal[V]) closeAll() {
	b.broadcaster.closeAll()
}

// Read returns the current signal value.
func (b *baseSignal[V]) Read() V {
	b.mu.RLock()
	v := b.value
	b.mu.RUnlock()
	return v
}

// Subscribe returns a channel that receives changed signal values.
func (b *baseSignal[V]) Subscribe(ctx context.Context) <-chan V {
	b.mu.RLock()
	current := b.value
	initial := b.initialNotify
	b.mu.RUnlock()
	return b.broadcaster.subscribeValues(ctx, current, initial)
}

// Watch returns a channel that receives a notification when the signal changes.
func (b *baseSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	b.mu.RLock()
	initial := b.initialNotify
	b.mu.RUnlock()
	return b.broadcaster.subscribeNotifications(ctx, initial)
}
