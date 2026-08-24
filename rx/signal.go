package rx

import (
	"context"
	"sync"
)

// Dependency is a named reactive value that publishes change notifications.
// It is intentionally not generic so a Derived may depend on values of
// different types. Watch must close its channel when the context is canceled.
type Dependency interface {
	ID() string
	Watch(context.Context) <-chan struct{}
}

// Readable is reactive state that can be read and independently subscribed to.
type Readable[V any] interface {
	Dependency

	// Value returns the current value.
	Value() V

	// Subscribe returns a coalescing stream that starts with the current value.
	Subscribe(context.Context) <-chan V
}

// Writable is readable reactive state that can also be set.
type Writable[V any] interface {
	Readable[V]

	// Set replaces the current value and publishes it to every subscriber.
	Set(V)
}

// Signal is passive, concurrency-safe reactive state.
//
// Every Set is a publication, including when the new value equals the current
// value. Subscribers are independent and coalesce pending publications to the
// latest value, so a slow subscriber never blocks Set.
type Signal[V any] struct {
	id                string
	value             V
	valueSubscribers  map[chan V]struct{}
	changeSubscribers map[chan struct{}]struct{}
	mu                sync.RWMutex
}

// NewSignal creates a signal with the given id and initial value.
func NewSignal[V any](id string, initial V) *Signal[V] {
	if id == "" {
		panic("rx: signal id cannot be empty")
	}

	return &Signal[V]{
		id:    id,
		value: initial,
	}
}

// ID returns the signal id.
func (s *Signal[V]) ID() string {
	return s.id
}

// Value returns the current signal value.
func (s *Signal[V]) Value() V {
	s.mu.RLock()
	value := s.value
	s.mu.RUnlock()
	return value
}

// Set replaces the current value and publishes it to every subscriber.
//
// Publications are serialized. Each subscriber retains only its latest pending
// value, so Set never waits for subscribers.
func (s *Signal[V]) Set(value V) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.value = value
	for subscriber := range s.valueSubscribers {
		sendLatest(subscriber, value)
	}

	for subscriber := range s.changeSubscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// Subscribe returns an independent capacity-one stream of signal values.
//
// The current value is sent first. Later publications are coalesced to the
// latest pending value. The channel closes when ctx is canceled.
func (s *Signal[V]) Subscribe(ctx context.Context) <-chan V {
	subscriber := make(chan V, 1)

	s.mu.Lock()

	if ctx.Err() != nil {
		close(subscriber)
		s.mu.Unlock()
		return subscriber
	}

	subscriber <- s.value
	if s.valueSubscribers == nil {
		s.valueSubscribers = make(map[chan V]struct{})
	}
	s.valueSubscribers[subscriber] = struct{}{}

	s.mu.Unlock()

	context.AfterFunc(ctx, func() {
		s.mu.Lock()
		if _, exists := s.valueSubscribers[subscriber]; exists {
			delete(s.valueSubscribers, subscriber)
			close(subscriber)
		}
		s.mu.Unlock()
	})

	return subscriber
}

// Watch returns an independent capacity-one stream of change notifications.
//
// An initial notification is available immediately. Later notifications are
// coalesced while one is pending. The channel closes when ctx is canceled.
func (s *Signal[V]) Watch(ctx context.Context) <-chan struct{} {
	subscriber := make(chan struct{}, 1)

	s.mu.Lock()

	if ctx.Err() != nil {
		close(subscriber)
		s.mu.Unlock()
		return subscriber
	}

	subscriber <- struct{}{}
	if s.changeSubscribers == nil {
		s.changeSubscribers = make(map[chan struct{}]struct{})
	}
	s.changeSubscribers[subscriber] = struct{}{}

	s.mu.Unlock()

	context.AfterFunc(ctx, func() {
		s.mu.Lock()
		if _, exists := s.changeSubscribers[subscriber]; exists {
			delete(s.changeSubscribers, subscriber)
			close(subscriber)
		}
		s.mu.Unlock()
	})

	return subscriber
}

func sendLatest[V any](output chan V, value V) {
	select {
	case output <- value:
		return
	default:
	}

	select {
	case <-output:
	default:
	}

	// Only this package sends on output. After either draining the pending value
	// or racing with its receiver, the capacity-one channel has room.
	output <- value
}
