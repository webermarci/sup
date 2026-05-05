# sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**sup** is a high-performance, low-allocation Actor Model library for Go.

It provides a robust foundation for building highly concurrent, distributed, and fault-tolerant stateful applications. It achieves very low allocations for asynchronous messages (`Cast`) and minimizes overhead for synchronous requests (`Call`) by utilizing typed inboxes and internal pooling. It embraces standard Go idioms (`select`, channels, and `context`) rather than hiding them behind heavy frameworks.

## Features

- **Idiomatic Go** — Actors are just goroutines running a `Run` loop. No magic interfaces, no reflection, no global registries.
- **OTP-style supervision** — Supervisor trees with `Permanent`, `Transient`, and `Temporary` restart policies.
- **Panic recovery** — Panics are caught, wrapped with a stack trace, and reported via `WithOnError`. The actor is then restarted according to the policy.
- **Restart limits** — Cap restarts within a sliding time window with `WithRestartLimit`.
- **Context-driven lifecycle** — `context.Context` ensures actors shut down cleanly when the parent context is canceled.
- **Typed inboxes** — `CastInbox[T]` and `CallInbox[T, R]` provide type-safe, efficient messaging.
- **Supervisor observers** — Lightweight lifecycle hooks for metrics, logging, or diagnostics via `SupervisorObserver`.
- **Supervisor trees** — Supervisors implement the `Actor` interface, so they compose naturally.

## Installation

```bash
go get github.com/webermarci/sup
```

## Quick Start

This example demonstrates a simple `Counter` actor using a `CastInbox` for fire-and-forget increments and a `CallInbox` for request/reply reads.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/webermarci/sup"
)

// 1. Define internal messages
type incrementMsg struct{ amount int }
type getCountMsg struct{}

// 2. Define the actor with typed inboxes
type Counter struct {
	*sup.BaseActor
	Casts *sup.CastInbox[incrementMsg]
	Calls *sup.CallInbox[getCountMsg, int]
	count int
}

func NewCounter() *Counter {
	return &Counter{
		BaseActor: sup.NewBaseActor("counter"),
		Casts:     sup.NewCastInbox[incrementMsg](16),
		Calls:     sup.NewCallInbox[getCountMsg, int](8),
	}
}

// 3. Public API — callers never access the inbox directly
func (c *Counter) Increment(amount int) {
	_ = c.Casts.Cast(context.Background(), incrementMsg{amount: amount})
}

func (c *Counter) Get() (int, error) {
	return c.Calls.Call(context.Background(), getCountMsg{})
}

// 4. Actor run loop
func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
			
		case inc, ok := <-c.Casts.Receive():
			if !ok {
				return nil
			}
			c.count += inc.amount
			
		case req, ok := <-c.Calls.Receive():
			if !ok {
				return nil
			}
			req.Reply(c.count, nil)
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	counter := NewCounter()

	// Optional: create a logger for the supervisor (propagated to child actors)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	supervisor := sup.NewSupervisor("root",
		sup.WithActor(counter),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
		sup.WithRestartLimit(5, 10*time.Second),
		sup.WithOnError(func(actor sup.Actor, err error) {
			fmt.Printf("Actor %s failed with error: %v\n", actor.Name(), err)
		}),
		sup.WithLogger(logger),
	)

	go supervisor.Run(ctx)

	counter.Increment(10)
	counter.Increment(32)

	count, err := counter.Get(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Final count: %d\n", count)

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

## Inboxes (CastInbox / CallInbox)

`CastInbox[T]` and `CallInbox[T, R]` are the type-safe building blocks for actor communication:

- `NewCastInbox[T](size int)` — create a cast inbox for messages of type `T`.
- `NewCallInbox[T, R](size int)` — create a call inbox that sends `T` and expects `R`.

`CastInbox[T]` API:

- `Cast(ctx context.Context, message T) error` — enqueue a message (blocks until space or context expires).
- `TryCast(ctx context.Context, message T) error` — non-blocking attempt; returns `ErrCastInboxFull` if full (or `ctx.Err()` if ctx is done).
- `Receive() <-chan T` — read-only channel for the actor's run loop.
- `Close()`, `Len()`, `Cap()`.

`CallInbox[T, R]` API:

- `Call(ctx context.Context, message T) (R, error)` — send a request and wait for reply (or context cancellation).
- `Receive() <-chan CallRequest[T, R]` — incoming requests inside the actor; use `req.Reply(value, err)` to respond.
- `Close()`, `Len()`, `Cap()`.

### Sending variants (summary)

| Method | Behaviour on full inbox | Behaviour on closed inbox |
|---|---|---|
| `(*CastInbox).Cast` | Blocks until space or `ctx` done | Returns `ErrCastInboxClosed` |
| `(*CastInbox).TryCast` | Returns `ErrCastInboxFull` immediately (or `ctx.Err()` if ctx done) | Returns `ErrCastInboxClosed` |
| `(*CallInbox).Call` | Blocks until reply or `ctx` done | Returns `ErrCallInboxClosed` |


## Supervisor Trees

Supervisors implement the `Actor` interface, so they can be nested inside one another. Child supervisors/actors inherit the supervisor's logger when spawned.

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

For actors that don't need a mailbox or internal state, use `ActorFunc`. Note the function receives both the `context.Context` and a `*slog.Logger` so you can log directly from the actor.

```go
healthCheck := sup.ActorFunc("health", func(ctx context.Context, logger *slog.Logger) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ping(); err != nil {
				logger.Error("health check failed", "err", err)
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

`sup` exposes a minimal observer mechanism via `SupervisorObserver` and the `WithObserver` option. Observers receive small, optional callbacks for lifecycle events. Callbacks are invoked asynchronously and panics are recovered — observers cannot block or crash the supervisor.

- `OnActorRegistered(actor Actor)`
- `OnActorStarted(actor Actor)`
- `OnActorStopped(actor Actor, err error)`
- `OnActorRestarting(actor Actor, restartCount int, lastErr error)`
- `OnSupervisorTerminal(err error)`

```go
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
```

## Router

`Router[F any]` is a small, generic utility for distributing work across a fixed set of routees. It provides low-overhead selection strategies and several helpers for broadcasting or fan-out execution.

Construction

- `NewRouter[F any](strategy RouterStrategy, routees ...F) *Router[F]` — create a router with one of the built-in strategies.
- Strategies: `RoundRobin`, `Random`.

API

- `(*Router[F]).Len() int` — number of routees.
- `(*Router[F]).Next() F` — pick the next routee according to the router strategy.
- `(*Router[F]).Sticky(key uint64) F` — pick a routee deterministically from a key (useful for consistent hashing-like behaviour).
- `(*Router[F]).Broadcast(fn func(F))` — call `fn` synchronously for every routee.
- `(*Router[F]).FanOut(fn func(F))` — call `fn` in a separate goroutine for each routee.
- `(*Router[F]).FanOutWait(fn func(F))` — fan out and wait for all invocations to finish.
- `(*Router[F]).Retry(limit int, run func(F) error) error` — try `run` on up to `limit` routees until one succeeds; returns the last error if all fail.

### Example

```go
workers := []*Worker{w1, w2, w3}
router := sup.NewRouter(sup.RoundRobin, workers...)

// get the next worker (round-robin)
w := router.Next().Process(task)

// broadcast to all workers
router.Broadcast(func(w *Worker) {
	w.Process(task)
})

// fan-out and wait for all workers to finish
router.FanOutWait(func(w *Worker) {
	w.Process(task)
})

// retry with up to 3 different workers
err := router.Retry(3, func(w *Worker) error {
  return w.Process(task)
})
```

## Packages

- `sup` — Core supervisor and typed inbox implementations
- `sup/bus` — Higher-level abstractions for polling and controlling with automatic scheduling and change notifications
- `sup/exec` — Actor wrapper around `os/exec` for managing external processes as actors
- `sup/hub` — Generic load balancer and distribution utility for grouping multiple function signatures and calling them with various strategies
- `sup/mesh` — NATS-backed actors for pub/sub messaging with automatic connection management
- `sup/modbus` — Actor wrapper around Modbus connections (TCP/RTU/ASCII) for thread-safe hardware access with automatic reconnection
- `sup/mqtt` — Actor wrapper around MQTT clients (Paho) for publish/subscribe with automatic reconnects and subscription handling
- `sup/sse` — Actor wrapper around Server-Sent Events (SSE) for consuming real-time event streams with automatic reconnection and last-event-id tracking
- `sup/ui` — Real-time dashboard for visualizing and inspecting actors in your supervisor tree
- `sup/ws` — Actor wrapper around WebSocket connections for thread-safe communication with automatic reconnection

## Benchmark

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/sup
cpu: Apple M5
BenchmarkCallInbox_SingleWorker-10        3318741    346.1 ns/op     0 B/op    0 allocs/op
BenchmarkCallInbox_Contention-10          1000000    682.5 ns/op     0 B/op    0 allocs/op
BenchmarkCastInbox_SingleWorker-10       37592265     31.9 ns/op     0 B/op    0 allocs/op
BenchmarkCastInbox_Parallel-10           24781441     48.6 ns/op     0 B/op    0 allocs/op
BenchmarkCastInbox_TryCast-10           134381470      8.9 ns/op     0 B/op    0 allocs/op
BenchmarkOutbox_Emit/1-10               187332144      6.4 ns/op     0 B/op    0 allocs/op
BenchmarkOutbox_Emit/10-10               41823868     28.3 ns/op     0 B/op    0 allocs/op
BenchmarkOutbox_Emit/100-10               4646634    256.1 ns/op     0 B/op    0 allocs/op
BenchmarkOutbox_Subscribe-10            100000000     24.0 ns/op    49 B/op    0 allocs/op
BenchmarkOutbox_EmitFireAndForget-10    337680223      3.6 ns/op     0 B/op    0 allocs/op
BenchmarkRouter_Next_RoundRobin-10      714396448      1.7 ns/op     0 B/op    0 allocs/op
BenchmarkRouter_Next_Random-10          236811043      5.1 ns/op     0 B/op    0 allocs/op
BenchmarkRouter_Next_Parallel-10         30816442     39.7 ns/op     0 B/op    0 allocs/op
BenchmarkSupervisor_SpawnAndExit-10       1810195    661.4 ns/op   474 B/op   12 allocs/op
BenchmarkSupervisor_RestartCycle-10       1218945    980.3 ns/op   224 B/op    6 allocs/op
BenchmarkSupervisor_ParallelSpawn-10      1644014    757.4 ns/op   616 B/op   11 allocs/op
```
