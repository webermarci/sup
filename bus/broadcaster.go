package bus

import (
	"context"
	"sync"
)

type broadcaster[V any] struct {
	mu               sync.RWMutex
	valueSubs        []chan V
	notificationSubs []chan struct{}
	buffer           int
}

func (b *broadcaster[V]) subscribeValues(ctx context.Context, initial V, notify bool) <-chan V {
	ch := make(chan V, b.buffer)

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
		defer b.mu.Unlock()
		for i, sub := range b.valueSubs {
			if sub == ch {
				b.valueSubs = append(b.valueSubs[:i], b.valueSubs[i+1:]...)
				close(ch)
				break
			}
		}
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
		defer b.mu.Unlock()
		for i, sub := range b.notificationSubs {
			if sub == ch {
				b.notificationSubs = append(b.notificationSubs[:i], b.notificationSubs[i+1:]...)
				close(ch)
				break
			}
		}
	})

	return ch
}

func (b *broadcaster[V]) notify(value V) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.valueSubs {
		select {
		case ch <- value:
		default:
		}
	}

	for _, ch := range b.notificationSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (b *broadcaster[V]) closeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.valueSubs {
		close(ch)
	}

	for _, ch := range b.notificationSubs {
		close(ch)
	}

	b.valueSubs = nil
	b.notificationSubs = nil
}
