# Sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**Sup** is zero-allocation, statically-typed Actor Model library for Go.

It provides a robust foundation for building highly concurrent, distributed, and fault-tolerant stateful applications without the overhead of the garbage collector getting in your way.

## Features

- **Statically Typed**: Leverages Go generics so you never have to type-assert interface responses. Your compiler knows exactly what your actors send and receive.
- **Zero Allocations**: Designed for the hot path. Message passing generates `0 B/op` and `0 allocs/op`.
- **OTP Semantics**: Supports asynchronous fire-and-forget messages (`Cast`) and synchronous request-reply transactions (`Call`).
- **Crash Recovery**: Actors are automatically supervised. If an actor panics, the supervisor catches it, cleans up the mailbox, and restarts the process from a clean state.
- **Thread-Safe Lifecycle**: Gracefully stop an actor from any goroutine natively, without channel deadlocks or race conditions.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"github.com/webermarci/sup"
)

// 1. Define your message types
type Increment struct{ Amount int }
type GetCount struct{}
type CounterState struct{ Total int }

// 2. Define your Actor
// Types: [ID type, Cast type, Call type, Reply type]
type CounterActor struct {
	sup.BaseActor[string, any, GetCount, CounterState]
	count int
}

// 3. Handle asynchronous fire-and-forget messages (Cast)
func (c *CounterActor) ReceiveCast(ctx context.Context, msg any) {
	switch m := msg.(type) {
	case Increment:
		c.count += m.Amount
	}
}

// 4. Handle synchronous request-reply messages (Call)
func (c *CounterActor) ReceiveCall(ctx context.Context, msg GetCount) (CounterState, error) {
	return CounterState{Total: c.count}, nil
}

// 5. Create a Producer function for the Supervisor
func NewCounter(id string) sup.Producer[string, any, GetCount, CounterState] {
	return func() sup.Actor[string, any, GetCount, CounterState] {
		return &CounterActor{
			BaseActor: sup.BaseActor[string, any, GetCount, CounterState]{ID: id},
			count:     0,
		}
	}
}

func main() {
	ctx := context.Background()
	sys := sup.NewSystem(ctx)

	// Spawn the actor under the system supervisor
	ref := sup.Spawn(sys, NewCounter("counter-1"))

	// Send asynchronous Casts (No blocking)
	ref.Cast(Increment{Amount: 10})
	ref.Cast(Increment{Amount: 32})

	// Send a synchronous Call (Blocks until reply is received)
	state, err := ref.Call(ctx, GetCount{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Final count: %d\n", state.Total) // Output: Final count: 42
}
```

## Benchmark

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/sup
cpu: Apple M5
BenchmarkActor_Cast-10             28562474    40.0 ns/op   0 B/op   0 allocs/op
BenchmarkActor_Call-10              2875939   405.7 ns/op   0 B/op   0 allocs/op
BenchmarkActor_ConcurrentCast-10   12235444    98.3 ns/op   0 B/op   0 allocs/op
BenchmarkActor_ConcurrentCall-10    1417048   869.3 ns/op   0 B/op   0 allocs/op
```
