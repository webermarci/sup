# sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**sup** is a high-performance, low-allocation Actor Model library for Go.

It provides a robust foundation for building highly concurrent, distributed, and fault-tolerant stateful applications. It achieves zero-allocation for asynchronous messages (`Cast`) and minimizes overhead for synchronous requests (`Call`) by utilizing internal resource pooling. It embraces standard Go idioms (`select`, channels, and `context`) rather than hiding them behind heavy frameworks.

## Features

- **Idiomatic Go** — Actors are just goroutines running a `select` loop. No magic interfaces, no reflection, no global registries.
- **OTP-style supervision** — Erlang-inspired supervisor trees with `Permanent`, `Transient`, and `Temporary` restart policies.
- **Panic recovery** — Panics are caught, wrapped with a stack trace, and reported via `WithOnError`. The actor is then restarted according to the policy.
- **Restart limits** — Optionally cap restarts within a sliding time window with `WithRestartLimit`.
- **Lifecycle callbacks** — React to restarts and errors via `WithOnRestart` and `WithOnError`.
- **No goroutine leaks** — `context.Context` integration ensures all actors shut down cleanly when the parent context is canceled.
- **Supervisor trees** — Supervisors implement the `Actor` interface, so they can be nested inside other supervisors.

## Installation

```bash
go get github.com/webermarci/sup
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/webermarci/sup"
)

// 1. Define internal messages
type incrementMsg struct{ amount int }
type getCountMsg struct{}

// 2. Define your Actor
type Counter struct {
	*sup.Mailbox
	count int
}

func NewCounter() *Counter {
	return &Counter{Mailbox: sup.NewMailbox(10)}
}

// 3. Clean public API — callers never interact with the mailbox directly
func (c *Counter) Increment(amount int) {
	_ = sup.Cast(c.Mailbox, incrementMsg{amount: amount})
}

func (c *Counter) Get() (int, error) {
	return sup.Call[getCountMsg, int](c.Mailbox, getCountMsg{})
}

// 4. The Run loop is a standard Go select statement
func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-c.Receive():
			switch m := msg.(type) {
			case sup.CastRequest[incrementMsg]:
				c.count += m.Payload().amount
			case sup.CallRequest[getCountMsg, int]:
				m.Reply(c.count, nil)
			}
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	counter := NewCounter()

	supervisor := sup.NewSupervisor(
		sup.WithActor(counter),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
		sup.WithRestartLimit(5, 10*time.Second),
		sup.WithOnError(func(err error) {
			fmt.Printf("actor error: %v\n", err)
		}),
	)

	go supervisor.Run(ctx)

	counter.Increment(10)
	counter.Increment(32)

	count, err := counter.Get()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Final count: %d\n", count) // Final count: 42

	cancel()
	supervisor.Wait()
}
```

## Restart Policies

| Policy | Clean exit (`nil`) | Error or panic |
|---|---|---|
| `Permanent` | Restarts | Restarts |
| `Transient` | Stops | Restarts |
| `Temporary` | Stops | Stops |

## Mailbox

A `Mailbox` is the actor's message queue. Messages are sent using `Cast` (fire-and-forget) or `Call` (request-reply), and received inside the actor's `Run` loop via `Receive()`.

### Sending variants

| Function | Behaviour on full mailbox | Behaviour on closed mailbox |
|---|---|---|
| `Cast` | Blocks until space is available | Returns `ErrMailboxClosed` |
| `CastContext` | Blocks until space or context expires | Returns `ErrMailboxClosed` |
| `TryCast` | Returns `ErrMailboxFull` immediately | Returns `ErrMailboxClosed` |
| `Call` | Blocks until reply is received | Returns `ErrMailboxClosed` |
| `CallContext` | Blocks until reply or context expires | Returns `ErrMailboxClosed` |
| `TryCall` | Returns `ErrMailboxFull` immediately | Returns `ErrMailboxClosed` |

## Supervisor Trees

Supervisors implement the `Actor` interface, so they compose naturally into trees. When the root context is canceled, shutdown propagates recursively through the entire tree.

```go
dbActor := NewDatabaseActor()
cacheActor := NewCacheActor()

// Child supervisor manages data-layer actors
dataSup := sup.NewSupervisor(
	sup.WithActors(dbActor, cacheActor),
	sup.WithPolicy(sup.Permanent),
	sup.WithRestartDelay(500*time.Millisecond),
)

// Root supervisor treats the child supervisor as an actor
root := sup.NewSupervisor(
	sup.WithActor(dataSup),
	sup.WithPolicy(sup.Permanent),
)

go root.Run(ctx)
```

## Stateless Actors

For actors that don't need a mailbox or internal state, use `ActorFunc`:

```go
healthCheck := sup.ActorFunc(func(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ping(); err != nil {
				return err // supervisor will restart based on policy
			}
		}
	}
})

sup.NewSupervisor(
	sup.WithActor(healthCheck),
	sup.WithPolicy(sup.Transient),
).Run(ctx)
```

## Dynamic Spawning

Use `Spawn` to start actors dynamically after the supervisor is already running:

```go
supervisor := sup.NewSupervisor(
	sup.WithPolicy(sup.Temporary),
)
go supervisor.Run(ctx)

// Later, spawn actors on demand
for _, job := range jobs {
	supervisor.Spawn(ctx, newJobActor(job))
}

supervisor.Wait()
```

## Packages

- [`sup`](./) — Core supervisor and mailbox implementation
- [`sup/bus`](./bus) — Higher-level abstractions for polling and controlling with automatic scheduling and change notifications

## Benchmark

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/sup
cpu: Apple M5
Benchmark_Cast-10                       20938254    57.2 ns/op     0 B/op   0 allocs/op
Benchmark_Cast_Concurrent-10            10266757   122.5 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext-10                21600847    55.9 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext_Concurrent-10     15451960    74.3 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext_Expired-10        24459242    48.9 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast-10                   192998186     6.2 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast_Concurrent-10         85340160    14.9 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast_Full-10              244051416     4.9 ns/op     0 B/op   0 allocs/op
Benchmark_Call-10                        3548784   341.7 ns/op    16 B/op   1 allocs/op
Benchmark_Call_Concurrent-10             2394580   501.9 ns/op    16 B/op   1 allocs/op
Benchmark_CallContext-10                 3018014   397.2 ns/op    16 B/op   1 allocs/op
Benchmark_CallContext_Concurrent-10      1422726   842.3 ns/op    16 B/op   1 allocs/op
Benchmark_CallContext_Expired-10        19289618    62.2 ns/op    16 B/op   1 allocs/op
Benchmark_TryCall-10                     3556698   341.0 ns/op    16 B/op   1 allocs/op
Benchmark_TryCall_Concurrent-10          3028040   453.5 ns/op    16 B/op   1 allocs/op
Benchmark_Supervisor_SpawnAndExit-10     4608442   261.4 ns/op    72 B/op   2 allocs/op
```
