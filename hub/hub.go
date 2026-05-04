package hub

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Hub is a simple load balancer that distributes work across a set of routees (workers).
// It supports different routing strategies, such as round-robin and random selection.
// The Next method returns the next routee according to the configured strategy.
// The Broadcast method applies a function to all routees sequentially, while FanOut does so concurrently.
type Hub[F any] struct {
	routees []F
	next    func() F
}

// NewRoundRobin creates a new Hub that uses round-robin routing to distribute work across the given routees.
func NewRoundRobin[F any](routees ...F) *Hub[F] {
	if len(routees) == 0 {
		panic("hub: at least one routee is required")
	}

	var counter uint64
	n := uint64(len(routees))

	return &Hub[F]{
		routees: routees,
		next: func() F {
			i := atomic.AddUint64(&counter, 1)
			return routees[(i-1)%n]
		},
	}
}

// NewRandom creates a new Hub that uses random selection to distribute work across the given routees.
func NewRandom[F any](routees ...F) *Hub[F] {
	if len(routees) == 0 {
		panic("hub: at least one routee is required")
	}

	n := len(routees)

	return &Hub[F]{
		routees: routees,
		next: func() F {
			return routees[rand.IntN(n)]
		},
	}
}

// Len returns the number of routees in the Hub.
func (h *Hub[F]) Len() int {
	return len(h.routees)
}

// Next returns the next routee according to the configured routing strategy.
func (h *Hub[F]) Next() F {
	return h.next()
}

// Sticky returns a routee based on the given key,
// using modulo to ensure it falls within the range of available routees.
func (h *Hub[F]) Sticky(key uint64) F {
	return h.routees[key%uint64(len(h.routees))]
}

// Broadcast applies the given function to all routees sequentially.
func (h *Hub[F]) Broadcast(fn func(F)) {
	for _, routee := range h.routees {
		fn(routee)
	}
}

// FanOut applies the given function to all routees concurrently.
func (h *Hub[F]) FanOut(fn func(F)) {
	for _, routee := range h.routees {
		go fn(routee)
	}
}

// FanOutWait applies the given function to all routees concurrently
// and waits for all of them to complete before returning.
func (h *Hub[F]) FanOutWait(fn func(F)) {
	var wg sync.WaitGroup
	wg.Add(len(h.routees))
	for _, routee := range h.routees {
		go func(r F) {
			defer wg.Done()
			fn(r)
		}(routee)
	}
	wg.Wait()
}

// Retry attempts to execute a function against routees until it succeeds or the limit is reached.
// It returns the last error encountered if all attempts fail.
func (h *Hub[F]) Retry(limit int, run func(F) error) error {
	var lastErr error

	for range limit {
		routee := h.Next()

		if err := run(routee); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return lastErr
}
