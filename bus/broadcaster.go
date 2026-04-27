package bus

import (
	"context"

	"github.com/webermarci/sup"
)

type subscribeMessage[V any] struct {
	ch chan V
}

type unsubscribeMessage[V any] struct {
	ch chan V
}

type broadcaster[V any] struct {
	subs   []chan V
	buffer int
}

func (b *broadcaster[V]) add(ch chan V) {
	b.subs = append(b.subs, ch)
}

func (b *broadcaster[V]) remove(ch chan V) {
	for i, sub := range b.subs {
		if sub == ch {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			break
		}
	}
	close(ch)
}

func (b *broadcaster[V]) notify(v V) {
	for _, ch := range b.subs {
		select {
		case ch <- v:
		default:
		}
	}
}

func (b *broadcaster[V]) closeAll() {
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = b.subs[:0]
}

func (b *broadcaster[V]) subscribe(ctx context.Context, mailbox *sup.Mailbox) <-chan V {
	ch := make(chan V, b.buffer)

	if _, err := sup.Call[subscribeMessage[V], error](mailbox, subscribeMessage[V]{ch: ch}); err != nil {
		close(ch)
		return ch
	}

	context.AfterFunc(ctx, func() {
		if err := sup.Cast(mailbox, unsubscribeMessage[V]{ch: ch}); err != nil {
			close(ch)
		}
	})

	return ch
}
