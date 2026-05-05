package sup

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Router provides various strategies for routing messages to a set of routees (e.g., actors, workers, etc.). It supports round-robin and random routing, as well as sticky routing based on a key. Additionally, it offers methods for broadcasting messages to all routees and retrying operations with a specified limit.
type RouterStrategy uint8

const (
	RoundRobin RouterStrategy = iota
	Random
)

// Router is a generic type that manages a set of routees and provides methods to select one based on the configured strategy. It also supports broadcasting messages to all routees and retrying operations with a limit.
type Router[F any] struct {
	routees []F
	next    func() F
}

// NewRouter creates a new Router with the specified routing strategy and routees. It panics if no routees are provided or if an unknown strategy is specified.
func NewRouter[F any](strategy RouterStrategy, routees ...F) *Router[F] {
	if len(routees) == 0 {
		panic("sup: at least one routee is required")
	}

	switch strategy {
	case Random:
		n := len(routees)
		return &Router[F]{
			routees: routees,
			next: func() F {
				return routees[rand.IntN(n)]
			},
		}

	case RoundRobin:
		var counter uint64
		n := uint64(len(routees))
		return &Router[F]{
			routees: routees,
			next: func() F {
				i := atomic.AddUint64(&counter, 1)
				return routees[(i-1)%n]
			},
		}

	default:
		panic("sup: unknown routing strategy")
	}
}

// Len returns the number of routees managed by the router.
func (r *Router[F]) Len() int {
	return len(r.routees)
}

// Next returns the next routee based on the configured routing strategy.
func (r *Router[F]) Next() F {
	return r.next()
}

// Sticky returns a routee based on the provided key, ensuring that the same key always maps to the same routee. This is useful for scenarios where you want to maintain affinity between certain messages and specific routees.
func (r *Router[F]) Sticky(key uint64) F {
	return r.routees[key%uint64(len(r.routees))]
}

// Broadcast applies the provided function to all routees sequentially. This is useful for sending the same message or performing the same action on all routees.
func (r *Router[F]) Broadcast(fn func(F)) {
	for _, routee := range r.routees {
		fn(routee)
	}
}

// FanOut applies the provided function to all routees concurrently. This is useful for performing actions on all routees in parallel, but it does not wait for the operations to complete.
func (r *Router[F]) FanOut(fn func(F)) {
	for _, routee := range r.routees {
		go fn(routee)
	}
}

// FanOutWait applies the provided function to all routees concurrently and waits for all operations to complete before returning. This is useful when you need to ensure that all routees have processed the message or action before proceeding.
func (r *Router[F]) FanOutWait(fn func(F)) {
	var wg sync.WaitGroup
	wg.Add(len(r.routees))
	for _, routee := range r.routees {
		go func(r F) {
			defer wg.Done()
			fn(r)
		}(routee)
	}
	wg.Wait()
}

// Retry attempts to execute the provided function with a routee up to the specified limit. If the function returns an error, it will retry with the next routee until the limit is reached. It returns the last error encountered if all attempts fail.
func (r *Router[F]) Retry(limit int, run func(F) error) error {
	var lastErr error

	for range limit {
		routee := r.Next()

		if err := run(routee); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return lastErr
}
