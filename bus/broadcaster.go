package bus

import (
	"context"
	"sync"
)

type broadcaster[V any] struct {
	mu     sync.RWMutex
	subs   []chan V
	buffer int
}

func (b *broadcaster[V]) subscribe(ctx context.Context, initial V, notify bool) <-chan V {
	ch := make(chan V, b.buffer)

	b.mu.Lock()
	b.subs = append(b.subs, ch)
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
		for i, sub := range b.subs {
			if sub == ch {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				close(ch)
				break
			}
		}
	})

	return ch
}

func (b *broadcaster[V]) notify(v V) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- v:
		default:
		}
	}
}

func (b *broadcaster[V]) closeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}
