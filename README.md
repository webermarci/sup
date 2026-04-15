# Sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**Sup** is a highly optimized, zero-allocation Actor Model library for Go.

It provides a robust foundation for building highly concurrent, distributed, and fault-tolerant stateful applications without the overhead of the garbage collector getting in your way. It embraces standard Go idioms (`select`, channels, and `context`) rather than hiding them behind heavy frameworks.

## Features

- **Idiomatic Go**: Actors are just standard Goroutines running a `select` loop. No magic interfaces, no reflection, no global registries.
- **Zero Allocations**: Designed for the hot path. Under the hood, `sup` uses generic `sync.Pool` structures to ensure synchronous message passing generates `0 allocs/op` on the heap.
- **OTP Supervision**: Built-in Erlang-style Supervisor trees. If an actor panics, the supervisor catches it and restarts it based on your defined policy (`Permanent`, `Temporary`, `Transient`).
- **Type-Safe**: Leverages Go generics for `Call` requests, ensuring your compiler knows exactly what your actors reply with.
- **No Goroutine Leaks**: `context.Context` integration ensures all actors gracefully shut down when their parent context is canceled.

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/webermarci/sup"
)

// 1. Define internal messages (unexported so they are hidden from the API)
type incrementMsg struct{ amount int }
type getCountMsg struct{}

// 2. Define your Actor
type Counter struct {
	// Embed the Mailbox.
	*sup.Mailbox[any]
	count int
}

func NewCounter() *Counter {
	return &Counter{
		Mailbox: sup.NewMailbox[any](10),
	}
}

// 3. Clean API Methods (Encapsulation)
// The caller never needs to know about Cast, Call, or Mailboxes!

func (c *Counter) Increment(amount int) {
	// Fire and forget
	_ = c.Cast(incrementMsg{amount: amount})
}

func (c *Counter) Get() (int, error) {
	// Synchronous request-reply
	return sup.Call[getCountMsg, int](ctx, c.Mailbox, getCountMsg{})
}

// 4. The Actor's Run loop is just a standard Go select statement
func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done(): // Graceful shutdown
			return ctx.Err()

		case msg := <-c.Receive():
			switch m := msg.(type) {
			case incrementMsg:
				c.count += m.amount
			case sup.Request[getCountMsg, int]:
				m.Reply(c.count, nil)
			}
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the counter
	counter := NewCounter()

	// Start the actor under a Supervisor
	supervisor := &sup.Supervisor{Policy: sup.Permanent}
	supervisor.Go(ctx, counter.Run)

	// --- Use the clean, thread-safe API ---
	counter.Increment(10)
	counter.Increment(32)

	count, err := counter.Get()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Final count: %d\n", count) // Output: Final count: 42
	
	// Shut down the supervisor and wait for actors to exit cleanly
	cancel()
	supervisor.Wait()
}
```

## Benchmark

Sup achieves extreme performance by utilizing lock-free atomic state, direct channel references, and generic `sync.Pool` channel reuse.

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/sup
cpu: Apple M5
BenchmarkMailbox_TryCast-10             568021905     3.86 ns/op    0 B/op   0 allocs/op
BenchmarkMailbox_ConcurrentTryCast-10   130512900    11.19 ns/op    0 B/op   0 allocs/op
BenchmarkActor_PingPongTryCast-10         7121670   173.82 ns/op    0 B/op   0 allocs/op
```
