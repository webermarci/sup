# Sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**Sup** is a highly optimized, zero-allocation Actor Model library for Go.

It provides a robust foundation for building highly concurrent, distributed, and fault-tolerant stateful applications without the overhead of the garbage collector getting in your way. It embraces standard Go idioms (`select`, channels, and `context`) rather than hiding them behind heavy frameworks.

## Features

- **Idiomatic Go**: Actors are just standard Goroutines running a `select` loop. No magic interfaces, no reflection, no global registries.
- **OTP Supervision**: Built-in Erlang-style Supervisor trees. If an actor panics, the supervisor catches it and restarts it based on your defined policy (`Permanent`, `Temporary`, `Transient`).
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
	*sup.Mailbox
	count int
}

func NewCounter() *Counter {
	return &Counter{
		Mailbox: sup.NewMailbox(10),
	}
}

// 3. Clean API Methods (Encapsulation)
// The caller never needs to know about Cast, Call, or Mailboxes!

func (c *Counter) Increment(amount int) {
	// Fire and forget
	_ = sup.Cast(c.Mailbox, incrementMsg{amount: amount})
}

func (c *Counter) Get() (int, error) {
	// Synchronous request-reply
	return sup.Call[getCountMsg, int](c.Mailbox, getCountMsg{})
}

// 4. The Actor's Run loop is just a standard Go select statement
func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done(): // Graceful shutdown
			return ctx.Err()

		case msg := <-c.Receive():
			switch m := msg.(type) {
			case *sup.CastRequest[incrementMsg]:
				c.count += m.Payload().amount
			case *sup.CallRequest[getCountMsg, int]:
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

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/sup
cpu: Apple M5
Benchmark_Cast-10                       20505252    58.0 ns/op     0 B/op   0 allocs/op
Benchmark_Cast_Concurrent-10            10264411   119.0 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext-10                21542421    55.8 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext_Concurrent-10     16066012    79.7 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext_Expired-10        24856513    48.5 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast-10                   215840746     5.5 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast_Concurrent-10         90003376    13.9 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast_Full-10              276174537     4.4 ns/op     0 B/op   0 allocs/op
Benchmark_Call-10                        3143728   384.3 ns/op   152 B/op   3 allocs/op
Benchmark_Call_Concurrent-10             2299144   555.3 ns/op   152 B/op   3 allocs/op
Benchmark_CallContext-10                 2709127   444.2 ns/op   152 B/op   3 allocs/op
Benchmark_CallContext_Concurrent-10      1327964   894.0 ns/op   152 B/op   3 allocs/op
Benchmark_CallContext_Expired-10        15570664    76.9 ns/op   152 B/op   3 allocs/op
Benchmark_TryCall-10                     3156140   379.3 ns/op   152 B/op   3 allocs/op
Benchmark_TryCall_Concurrent-10          2286416   521.7 ns/op   152 B/op   3 allocs/op
Benchmark_PingPong_Cast-10               5967357   201.5 ns/op    24 B/op   1 allocs/op
Benchmark_PingPong_CastContext-10        5916985   203.2 ns/op    24 B/op   1 allocs/op
Benchmark_PingPong_TryCast-10            5941306   201.6 ns/op    24 B/op   1 allocs/op
Benchmark_Supervisor_SpawnAndExit-10     4646175   259.4 ns/op    72 B/op   2 allocs/op
```
