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
- **No goroutine leaks** — `context.Context` integration ensures all actors shut down cleanly when the parent context is canceled.
- **Supervisor observers** — Lightweight lifecycle hooks so you can collect metrics, log events, or build diagnostics without changing supervision semantics.
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
	*sup.BaseActor
	mailbox *sup.Mailbox
	count int
}

func NewCounter() *Counter {
	return &Counter{
		BaseActor: sup.NewBaseActor("counter"),
		mailbox: sup.NewMailbox(10),
	}
}

// 3. Clean public API — callers never interact with the mailbox directly
func (c *Counter) Increment(amount int) {
	_ = sup.Cast(c.mailbox, incrementMsg{amount: amount})
}

func (c *Counter) Get() (int, error) {
	return sup.Call[getCountMsg, int](c.mailbox, getCountMsg{})
}

// 4. The Run loop is a standard Go select statement
func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-c.mailbox.Receive():
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

	supervisor := sup.NewSupervisor("root",
		sup.WithActor(counter),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
		sup.WithRestartLimit(5, 10*time.Second),
		sup.WithOnError(func(actor sup.Actor, err error) {
			fmt.Printf("Actor %s failed with error: %v\n", actor.Name(), err)
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
dataSup := sup.NewSupervisor("data_supervisor",
	sup.WithActors(dbActor, cacheActor),
	sup.WithPolicy(sup.Permanent),
	sup.WithRestartDelay(500*time.Millisecond),
)

// Root supervisor treats the child supervisor as an actor
root := sup.NewSupervisor("root",
	sup.WithActor(dataSup),
	sup.WithPolicy(sup.Permanent),
)

go root.Run(ctx)
```

## Stateless Actors

For actors that don't need a mailbox or internal state, use `ActorFunc`:

```go
healthCheck := sup.ActorFunc("health", func(ctx context.Context) error {
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

sup.NewSupervisor("health_supervisor",
	sup.WithActor(healthCheck),
	sup.WithPolicy(sup.Transient),
).Run(ctx)
```

## Dynamic Spawning

Use `Spawn` to start actors dynamically after the supervisor is already running:

```go
supervisor := sup.NewSupervisor("job_supervisor",
	sup.WithPolicy(sup.Temporary),
)
go supervisor.Run(ctx)

// Later, spawn actors on demand
for _, job := range jobs {
	supervisor.Spawn(ctx, newJobActor(job))
}

supervisor.Wait()
```

## Observability

`sup` exposes a minimal, flexible observer mechanism via `SupervisorObserver` and the `WithObserver` option. Observers are small collections of optional callbacks that receive lifecycle events from the supervisor:

- `OnActorRegistered(actor Actor)` — called when `Spawn` is invoked for an actor.
- `OnActorStarted(actor Actor)` — called immediately before `actor.Run(ctx)` for each run.
- `OnActorStopped(actor Actor, err error)` — called after `actor.Run` returns (error may be nil for clean exits).
- `OnActorRestarting(actor Actor, restartCount int, lastErr error)` — called when the supervisor decides to restart an actor.
- `OnSupervisorTerminal(err error)` — called when the supervisor escalates to a terminal error (e.g. restart limits exceeded).

Design notes:
- All callbacks are optional — provide only the ones you need.
- Callbacks are invoked asynchronously (each runs in its own goroutine) and any panic inside an observer is recovered. Observers cannot block or crash the supervisor.
- Observers receive the `Actor` value so they may type-assert to access actor-specific fields (for example, bus actors may expose a `Mailbox()` accessor to inspect queue size).

```go
package main

import (
    "fmt"

    "github.com/webermarci/sup"
)

func main() {
	observer := &sup.SupervisorObserver{
		OnActorRegistered: func(a sup.Actor) {
			fmt.Printf("registered: %s\n", a.Name())
		},
		OnActorStarted: func(a sup.Actor) {
			fmt.Printf("started: %s\n", a.Name())
		},
		OnActorStopped: func(a sup.Actor, err error) {
			fmt.Printf("stopped: %s err=%v\n", a.Name(), err)
		},
		OnActorRestarting: func(a sup.Actor, count int, lastErr error) {
      fmt.Printf("restarting: %s count=%d lastErr=%v\n", a.Name(), count, lastErr)
    },
		OnSupervisorTerminal: func(err error) {
			fmt.Printf("supervisor terminal: err=%v\n", err)
		},
	}

	supervisor := sup.NewSupervisor("root",
		sup.WithObserver(observer),
	)
}
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
Benchmark_Cast-10                       20725142    57.4 ns/op     0 B/op   0 allocs/op
Benchmark_Cast_Concurrent-10            10265941   117.4 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext-10                21805774    55.2 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext_Concurrent-10     15253065    79.0 ns/op     0 B/op   0 allocs/op
Benchmark_CastContext_Expired-10        23459665    51.0 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast-10                   198711219     6.0 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast_Concurrent-10         89085205    13.9 ns/op     0 B/op   0 allocs/op
Benchmark_TryCast_Full-10              254923090     4.7 ns/op     0 B/op   0 allocs/op
Benchmark_Call-10                        3095778   390.2 ns/op   152 B/op   3 allocs/op
Benchmark_Call_Concurrent-10             2198695   506.8 ns/op   152 B/op   3 allocs/op
Benchmark_CallContext-10                 2545513   451.1 ns/op   152 B/op   3 allocs/op
Benchmark_CallContext_Concurrent-10      1301760   896.5 ns/op   152 B/op   3 allocs/op
Benchmark_CallContext_Expired-10        14855138    78.4 ns/op   152 B/op   3 allocs/op
Benchmark_TryCall-10                     3060210   392.7 ns/op   152 B/op   3 allocs/op
Benchmark_TryCall_Concurrent-10          2227587   527.4 ns/op   152 B/op   3 allocs/op
Benchmark_Supervisor_SpawnAndExit-10     4714188   236.9 ns/op    72 B/op   2 allocs/op
```
