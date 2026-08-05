package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
)

// Topic broadcasts each published value to every active subscription.
//
// Topic is passive: it does not need to be started or supervised. Its
// subscriptions are unbuffered, so Publish applies direct backpressure until
// every active subscriber receives the value.
type Topic[T any] struct {
	subscribers   map[*Subscription[T]]struct{}
	subscribersMu sync.RWMutex
}

// New creates a typed unbuffered topic.
func New[T any]() *Topic[T] {
	return &Topic[T]{
		subscribers: make(map[*Subscription[T]]struct{}),
	}
}

// Subscribe creates an independent subscription whose lifetime is bound to
// ctx. The returned channel closes when ctx is canceled.
func (t *Topic[T]) Subscribe(ctx context.Context) *Subscription[T] {
	if t == nil {
		panic("pubsub: cannot subscribe to a nil topic")
	}
	if ctx == nil {
		panic("pubsub: subscription context cannot be nil")
	}

	subscription := &Subscription[T]{
		topic:  t,
		values: make(chan T),
		done:   make(chan struct{}),
	}

	t.subscribersMu.Lock()
	t.subscribers[subscription] = struct{}{}
	t.subscribersMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			subscription.close()
		case <-subscription.done:
		}
	}()

	return subscription
}

// Publish sends value to every subscription that is active when publishing
// begins. A canceled context stops delivery and returns its error. Delivery to
// subscriptions that already received the value cannot be rolled back.
func (t *Topic[T]) Publish(ctx context.Context, value T) error {
	if t == nil {
		panic("pubsub: cannot publish to a nil topic")
	}
	if ctx == nil {
		panic("pubsub: publish context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	t.subscribersMu.RLock()
	subscribers := make([]*Subscription[T], 0, len(t.subscribers))
	for subscription := range t.subscribers {
		subscribers = append(subscribers, subscription)
	}
	t.subscribersMu.RUnlock()

	for _, subscription := range subscribers {
		if err := subscription.publish(ctx, value); err != nil {
			return err
		}
	}

	return nil
}

// SubscriberCount returns the number of currently active subscriptions.
func (t *Topic[T]) SubscriberCount() int {
	if t == nil {
		return 0
	}

	t.subscribersMu.RLock()
	count := len(t.subscribers)
	t.subscribersMu.RUnlock()
	return count
}

func (t *Topic[T]) remove(subscription *Subscription[T]) {
	t.subscribersMu.Lock()
	delete(t.subscribers, subscription)
	t.subscribersMu.Unlock()
}

// Subscription is one independent consumer of a Topic.
type Subscription[T any] struct {
	topic  *Topic[T]
	values chan T
	done   chan struct{}

	mu     sync.Mutex
	closed atomic.Bool
}

// Receive returns the subscription's read-only value stream.
func (s *Subscription[T]) Receive() <-chan T {
	if s == nil {
		return nil
	}
	return s.values
}

func (s *Subscription[T]) close() {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return
	}

	close(s.done)
	s.topic.remove(s)

	s.mu.Lock()
	close(s.values)
	s.mu.Unlock()
}

func (s *Subscription[T]) publish(ctx context.Context, value T) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return nil
	}

	select {
	case s.values <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return nil
	}
}
