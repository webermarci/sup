# sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**sup** is a small actor supervision toolkit for Go, with an optional reactive
signal package.

It provides typed inboxes for actor communication, OTP-style supervision with restart policies, reactive values that can be composed and observed, and an optional HTTP hub for inspecting actors, signals, supervision, and events.

The core package is intentionally small: actors own application behavior and
state, while supervisors own lifecycle and recovery. Start with the core
package and add `rx`, `hub`, or `process` only when your application needs
those capabilities.

## Features

- **Idiomatic actors** — An actor is any value that implements `ID()` and `Run(context.Context) error`.
- **Supervisor trees** — Supervisors are actors too, so they can be nested like ordinary actors.
- **Restart policies** — `Permanent`, `Transient`, and `Temporary` policies control when actors restart.
- **Panic recovery** — Panics are recovered, wrapped with a stack trace, reported, and handled by the restart policy.
- **Typed inboxes** — `CastInbox[T]` and `CallInbox[T, R]` provide type-safe asynchronous and request/reply messaging.
- **Reactive signals** — The `rx` package provides passive signals and supervised derived state.
- **Channel helpers** — `Map`, `Filter`, `Distinct`, `Debounce`, and explicit throttle helpers compose ordinary channels.
- **Runtime debugging** — Runtime events and `hub` expose actor state, supervision, and signals.

## Installation

```bash
go get github.com/webermarci/sup
```

The module requires Go 1.27 or newer.

## Start here

A typical application built with `sup` has this shape:

1. Define one or more actors that implement `ID` and `Run`.
2. Put the actors under a root `Supervisor`.
3. Expose actor operations through typed `CastInbox` and `CallInbox` values.
4. Run the root supervisor with the application's context.
5. Add `rx` for shared reactive state, `hub` for an HTTP inspection surface,
   `process` for supervised external commands, or `resource` for serialized
   access to a resource such as a serial connection.

The most important design rules are:

- Treat context cancellation as a normal shutdown and return `nil` from
  `Run`.
- Return a non-nil error only for a failure that should be visible to the
  supervisor and may require a restart.
- Keep mutable actor state owned by the actor's `Run` loop; use inboxes for
  concurrent callers to communicate with it.
- Do not close inboxes yourself. Their lifetime is controlled by the actor's
  context.
- `CallInbox` handlers should reply once and check the boolean result of
  `Reply`, because the caller may have canceled.
- Put every `rx.Derived` value that should update into a supervisor tree; a
  derived value is an actor, unlike a passive `rx.Signal`.

### Package map

| Package | Use it for |
| --- | --- |
| `github.com/webermarci/sup` | Actors, supervisors, typed inboxes, and runtime events |
| `github.com/webermarci/sup/rx` | Signals, derived state, and channel pipelines |
| `github.com/webermarci/sup/hub` | HTTP snapshots, live events, and optional signal controls |
| `github.com/webermarci/sup/httpserver` | Supervised standard-library HTTP servers |
| `github.com/webermarci/sup/process` | Running `os/exec.Cmd` values as supervised actors |
| `github.com/webermarci/sup/resource` | Supervised, serialized access to acquired resources |

For the API reference, see the [core package documentation](https://pkg.go.dev/github.com/webermarci/sup), [`rx`](https://pkg.go.dev/github.com/webermarci/sup/rx), [`hub`](https://pkg.go.dev/github.com/webermarci/sup/hub), [`httpserver`](https://pkg.go.dev/github.com/webermarci/sup/httpserver), [`process`](https://pkg.go.dev/github.com/webermarci/sup/process), and [`resource`](https://pkg.go.dev/github.com/webermarci/sup/resource) package pages. The repository also contains a minimal [simple example](examples/simple) and a complete [dashboard example](examples/dashboard).

## Quick start

This example defines a counter actor with a fire-and-forget increment inbox and a request/reply get inbox.

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
)

type GetMessage struct{}

type IncrementMessage struct {
	Amount int
}

type Counter struct {
	id             string
	GetInbox       *sup.CallInbox[GetMessage, int]
	IncrementInbox *sup.CastInbox[IncrementMessage]
	State          int
}

func NewCounter(id string) *Counter {
	return &Counter{
		id:             id,
		GetInbox:       sup.NewCallInbox[GetMessage, int](),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](),
	}
}

func (c *Counter) ID() string {
	return c.id
}

func (c *Counter) Get(ctx context.Context) (int, error) {
	return c.GetInbox.Call(ctx, GetMessage{})
}

func (c *Counter) Increment(ctx context.Context, amount int) error {
	return c.IncrementInbox.Cast(ctx, IncrementMessage{Amount: amount})
}

func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case req := <-c.GetInbox.Receive():
			req.Reply(c.State, nil)

		case msg := <-c.IncrementInbox.Receive():
			c.State += msg.Amount
		}
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	counter := NewCounter("counter")

	supervisor := sup.NewSupervisor("root", sup.Permanent).
		SetRestartDelay(time.Second).
		SetRestartLimit(5, 10*time.Second).
		AddActors(counter)

	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	_ = counter.Increment(ctx, 10)
	_ = counter.Increment(ctx, 32)

	count, err := counter.Get(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println("count:", count)

	cancel()
	if err := <-done; err != nil {
		fmt.Println("supervisor stopped:", err)
	}
}
```

## Actors

An actor implements the `Actor` interface:

```go
type Actor interface {
	ID() string
	Run(context.Context) error
}
```

Actor identity is ordinary application state:

```go
type Worker struct {
	id string
}

func NewWorker(id string) *Worker {
	return &Worker{id: id}
}

func (w *Worker) ID() string {
	return w.id
}
```

For stateless actors, use `ActorFunc`:

```go
worker := sup.ActorFunc("health", func(ctx context.Context) error {
	<-ctx.Done()
	return nil
})
```

### Lifecycle contract

Every actor follows the same `Run` contract:

- Context cancellation is a clean shutdown and returns `nil`.
- Intentional completion returns `nil`.
- A failure that may require a restart returns a non-nil error.
- A panic is recovered by the supervisor and treated as a failure.
- Each actor instance belongs to exactly one supervisor in a running tree.
- Complete all actor, supervisor, and hub configuration before running the tree.
- Applications call `Run` only on the root supervisor.
- Supervisors may call a child's `Run` again sequentially when restarting it,
  but the same actor instance must never run concurrently.

Supervisors follow the same actor contract, so they can be nested as ordinary
actors. A supervisor returns an error when a restart limit is exceeded.

## Supervisors

Supervisors run actors and restart them according to a restart policy.

```go
supervisor := sup.NewSupervisor("root", sup.Transient).
	SetRestartDelay(500 * time.Millisecond).
	SetRestartLimit(3, time.Minute).
	AddActors(actorA, actorB)

if err := supervisor.Run(ctx); err != nil {
	// A restart limit stopped the supervisor.
}
```

### Restart policies

| Policy | Clean exit (`nil`) | Error or panic |
|---|---:|---:|
| `Permanent` | restart | restart |
| `Transient` | stop | restart |
| `Temporary` | stop | stop |

`NewSupervisor` requires a restart policy and defaults to a one-second restart
delay with no restart limit. Configure the delay and limit explicitly when
restart timing or failure budgets are part of the application's behavior.
Use `SetRestartDelayFunc` when the application needs a different delay for each
restart, such as a capped exponential backoff:

```go
supervisor.SetRestartDelayFunc(func(restartCount int) time.Duration {
	if restartCount > 5 {
		restartCount = 5
	}
	return time.Duration(1<<uint(restartCount-1)) * time.Second
})
```

The function is called concurrently when multiple actors restart, and a
non-positive result means restart immediately.

### Runtime events

`EventHandler` is an optional actor capability. When an actor implements it, its
supervisor automatically sends it lifecycle events from that supervisor and
all descendants. Different actors may call event handlers concurrently, so
handlers should return promptly and synchronize their own state. Handler panics
are recovered and do not interrupt supervision.

```go
type EventLogger struct{}

func (*EventLogger) ID() string { return "events" }

func (*EventLogger) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (*EventLogger) HandleEvent(event sup.Event) {
	fmt.Printf("%s %s\n", event.Type, event.Actor.ID())

	if event.Err != nil {
		fmt.Println("error:", event.Err)
	}
}

root := sup.NewSupervisor("root", sup.Transient).
	AddActors(worker, &EventLogger{})
```

Each event carries its occurrence time, actor, supervisor, error, and restart
count as native Go values. `EventActorRegistered` marks the beginning of active
supervision during a `Run`. Presentation layers such as `hub` decide how to
serialize events. Multiple event-handler actors can independently record, log,
aggregate, or forward the same authoritative runtime event.

## Typed inboxes

`CastInbox[T]` is for asynchronous messages. `CallInbox[T, R]` is for request/reply interactions.

### Cast inbox

```go
inbox := sup.NewCastInbox[IncrementMessage]()

err := inbox.Cast(ctx, IncrementMessage{Amount: 1})    // blocks until received or ctx is done

for {
	select {
	case <-ctx.Done():
		return
	case msg := <-inbox.Receive():
		// process msg
	}
}
```

### Call inbox

```go
inbox := sup.NewCallInbox[GetMessage, int]()

value, err := inbox.Call(ctx, GetMessage{})

for {
	select {
	case <-ctx.Done():
		return
	case req := <-inbox.Receive():
		if !req.Reply(42, nil) {
			// The caller canceled or this request was already answered.
		}
	}
}
```

`CallRequest.Context` exposes the caller's context so work can stop after the
caller leaves. `Reply` never blocks and returns whether the reply was accepted.

Their lifetime is controlled by the actor's context; inboxes are not closed
independently.

## rx

The `github.com/webermarci/sup/rx` package provides shared reactive state and
composable channel helpers. It is a signal library, not an implementation of
ReactiveX.

### Signal

A `Signal[V]` is passive, concurrency-safe state with independent subscribers:

```go
count := rx.NewSignal("count", 0)

values := count.Subscribe(ctx)

go func() {
	for value := range values {
		fmt.Println(value)
	}
}()

count.Set(1)
fmt.Println(count.Value())
```

Use `Update` for an atomic read-modify-write:

```go
count.Update(func(current int) int {
	return current + 1
})
```

Every subscription starts with the current value and has capacity one. Pending
updates are coalesced to the latest value, so slow subscribers never block
`Set`. Each call to `Subscribe` creates an independent stream, and its channel
closes when the subscription context is canceled.

Every `Set` is a publication, even when it repeats the current value. Signals do
not run under a supervisor; actors own behavior and set signals:

```go
random := rx.NewSignal("random", 0)

poller := sup.ActorFunc("random.poller", func(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			random.Set(rand.IntN(100))
		case <-ctx.Done():
			return nil
		}
	}
})
```

### Channel helpers

Helpers accept and return ordinary channels, so they can be nested. They do not
need contexts: cancellation closes the subscription channel, and that closure
propagates through the pipeline.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
count := rx.NewSignal("count", 0)

processed := rx.Map(
	rx.Filter(
		rx.Debounce(count.Subscribe(ctx), 100*time.Millisecond),
		func(value int) bool { return value >= 10 },
	),
	func(value int) string { return strconv.Itoa(value) },
)
```

Available helpers:

- `Map` transforms values and may change their type.
- `Filter` drops values that do not match.
- `Distinct` drops consecutive equal comparable values.
- `DistinctFunc` uses a custom equality function.
- `Transitions` emits the previous and current values for each distinct change.
- `TransitionsFunc` detects transitions with a custom equality function.
- `Edges` emits rising and falling boolean transitions.
- `Debounce` emits after a quiet period.
- `ThrottleFirst` immediately emits the first value in each interval.
- `ThrottleLatest` emits the latest value at the end of each interval.

Helper outputs have capacity one and coalesce pending values to the latest.
They close when their input closes. Debounce and throttle durations must be
positive. `Debounce` and `ThrottleLatest` do not flush a pending value when
their input closes.

### Derived

`Derived[V]` is shared, read-only reactive state computed from one or more
dependencies. Unlike `Signal`, it is an actor because it actively maintains its
computed value:

```go
count := rx.NewSignal("count", 0)
doubled := rx.NewDerived("doubled", func() int {
	return count.Value() * 2
}, count)

root := sup.NewSupervisor("root", sup.Transient).AddActors(doubled)
go root.Run(ctx)
```

Signals and derived values both provide `ID`, `Value`, `Subscribe`, and `Watch`.
Only signals provide `Set`.

Dependency notifications already pending are coalesced. Updates to independent
dependencies are not transactional, so a derived signal may publish intermediate
results before converging on the latest state. Values that must change atomically
should be stored together in one signal.

Construction computes the initial snapshot. When `Run` starts, it registers every
dependency watch and publishes exactly one refreshed startup snapshot before
processing later dependency notifications. Derived dependency graphs must be
acyclic.

## Hub

The `github.com/webermarci/sup/hub` package exposes explicitly registered
actors and signals over HTTP. It reads signal values directly from their
sources, streams changes and lifecycle events, and builds a pruned supervision
graph from runtime registration events. Its HTML overview takes a fresh
snapshot through the same HTTP API on each page load, then applies live signal
and event updates from the SSE stream. It has no frontend dependencies.

```go
enabled := rx.NewSignal("counter_enabled", true)
status := rx.NewSignal("counter_status", "ready")

dashboard := hub.New("dashboard").
	RegisterActors(counter).
	RegisterSignal(status).
	RegisterWritableSignal(enabled).
	SetEventHistoryLimit(128)

server := httpserver.NewActor("dashboard.http",
	func(context.Context) (*http.Server, error) {
		return &http.Server{Addr: ":8080", Handler: dashboard}, nil
	},
).SetShutdownTimeout(5 * time.Second)

root := sup.NewSupervisor("root", sup.Transient).
	AddActors(counter, dashboard, server)

go root.Run(ctx)
```

The overview is served at `/`. The handler is prefix-agnostic, so applications
can also mount it below a path with `http.StripPrefix`.

`RegisterSignal` is always read-only over HTTP. `RegisterWritableSignal` explicitly
allows outside clients to change the signal with `PATCH`. Actors can subscribe
to that signal using the ordinary `rx` API; the hub does not define a separate
command or control model. Writable signals can also be changed with the forms
in the HTML overview, which call the same `PATCH` endpoint without reloading the
page.

Writable updates reject cross-origin browser requests, unknown JSON members,
and request bodies larger than 1 MiB. Requests from non-browser clients and
same-origin browser pages are allowed.

The hub does not provide authentication or authorization. If the handler is
reachable outside a trusted local environment, put it behind the application's
own authentication, authorization, and network controls—especially when using
`RegisterWritableSignal`.

```http
PATCH /signals/counter_enabled
Content-Type: application/json

{"value": false}
```

Only explicitly registered actors have their lifecycle events exposed. The
graph includes the supervisors needed to connect those actors to the root and
omits unrelated internal branches. Recent event history is globally bounded;
live event streaming is unaffected by the history limit.

Endpoints include:

- `GET /`
- `GET /signals`
- `GET /signals/{signalID}`
- `PATCH /signals/{signalID}`
- `GET /graph`
- `GET /events`
- `GET /events/stream`

Run the traffic-light example for a compact tour of the whole stack:

```bash
go run ./examples/dashboard
```

It combines a typed inbox, nested supervision, writable controls, derived
state, and live SSE updates. Change `cycle_length_seconds` to speed up or slow
down the intersection. Set `paused` to `true` to put the traffic light into a
stationary yellow state and turn the pedestrian signal off.

## Packages

- `sup` — Core actors, supervisors, typed inboxes, and runtime events.
- `sup/rx` — Reactive signals, derived state, and channel helpers.
- `sup/hub` — HTTP API for actors, signals, supervision, and events.
- `sup/httpserver` — Actor adapter for standard-library HTTP servers.
- `sup/process` — Actor adapter for `os/exec` commands.
- `sup/resource` — Serialized, supervised access to acquired resources.

For package-level documentation that is available through `go doc` and
`pkg.go.dev`, start with the package comments in `doc.go`, `rx/doc.go`,
`hub/doc.go`, `httpserver/doc.go`, `process/doc.go`, and `resource/doc.go`.
Examples in this README
are intended to be copied into an application and adapted to its own actors,
messages, and shutdown policy.
