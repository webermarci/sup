package sup

import (
	"context"
	"sync"
)

type broadcaster[V any] struct {
	buffer           int
	valueSubs        []chan V
	notificationSubs []chan struct{}
	mu               sync.RWMutex
}

func (b *broadcaster[V]) subscribeValues(ctx context.Context, initial V, notify bool) <-chan V {
	b.mu.RLock()
	buf := b.buffer
	b.mu.RUnlock()

	ch := make(chan V, buf)

	b.mu.Lock()
	b.valueSubs = append(b.valueSubs, ch)
	b.mu.Unlock()

	if notify {
		select {
		case ch <- initial:
		default:
		}
	}

	context.AfterFunc(ctx, func() {
		b.mu.Lock()
		for i, sub := range b.valueSubs {
			if sub == ch {
				b.valueSubs = append(b.valueSubs[:i], b.valueSubs[i+1:]...)
				close(ch)
				break
			}
		}
		b.mu.Unlock()
	})

	return ch
}

func (b *broadcaster[V]) subscribeNotifications(ctx context.Context, notify bool) <-chan struct{} {
	ch := make(chan struct{}, 1)

	b.mu.Lock()
	b.notificationSubs = append(b.notificationSubs, ch)
	b.mu.Unlock()

	if notify {
		select {
		case ch <- struct{}{}:
		default:
		}
	}

	context.AfterFunc(ctx, func() {
		b.mu.Lock()
		for i, sub := range b.notificationSubs {
			if sub == ch {
				b.notificationSubs = append(b.notificationSubs[:i], b.notificationSubs[i+1:]...)
				close(ch)
				break
			}
		}
		b.mu.Unlock()
	})

	return ch
}

func (b *broadcaster[V]) notify(value V) {
	b.mu.RLock()
	valueSubs := b.valueSubs
	notificationSubs := b.notificationSubs
	b.mu.RUnlock()

	for _, ch := range valueSubs {
		select {
		case ch <- value:
		default:
		}
	}

	for _, ch := range notificationSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (b *broadcaster[V]) closeAll() {
	b.mu.Lock()

	for _, ch := range b.valueSubs {
		close(ch)
	}

	for _, ch := range b.notificationSubs {
		close(ch)
	}

	b.valueSubs = nil
	b.notificationSubs = nil

	b.mu.Unlock()
}
