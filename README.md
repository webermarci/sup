# sup

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup.svg)](https://pkg.go.dev/github.com/webermarci/sup)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**sup** is a small actor supervision and reactive signal toolkit for Go.

It provides typed inboxes for actor communication, OTP-style supervision with restart policies, reactive values that can be composed and observed, and an optional HTTP hub for inspecting actors, controls, signals, and events.

## Features

- **Idiomatic actors** — An actor is any value that implements `ID()`, `Run(context.Context) error`, and `Inspect() Spec`.
- **Supervisor trees** — Supervisors are actors too, so they can supervise actors or other supervisors.
- **Restart policies** — `Permanent`, `Transient`, and `Temporary` policies control when actors restart.
- **Panic recovery** — Panics are recovered, wrapped with a stack trace, reported, and handled by the restart policy.
- **Typed inboxes** — `CastInbox[T]` and `CallInbox[T, R]` provide type-safe asynchronous and request/reply messaging.
- **Reactive signals** — `Signal`, `Derived`, and `Effect` model readable values, computed values, and side effects.
- **Signal processors** — Built-in `Map`, `Filter`, `Debounce`, and `Throttle` processors transform or rate-limit signal updates.
- **Runtime inspection** — `Spec`, controls, supervisor observers, and `hub` expose useful metadata for debugging and dashboards.

## Installation

```bash
go get github.com/webermarci/sup
```

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
	*sup.BaseActor
	GetInbox       *sup.CallInbox[GetMessage, int]
	IncrementInbox *sup.CastInbox[IncrementMessage]
	State          int
}

func NewCounter(id string) *Counter {
	return &Counter{
		BaseActor:      sup.NewBaseActor(id),
		GetInbox:       sup.NewCallInbox[GetMessage, int](8),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](8),
	}
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

	supervisor := sup.NewSupervisor("root").
		Policy(sup.Permanent).
		RestartDelay(time.Second).
		RestartLimit(5, 10*time.Second).
		OnError(func(actor sup.Actor, err error) {
			fmt.Printf("actor %s failed: %v\n", actor.ID(), err)
		}).
		Actor(counter)

	go func() {
		if err := supervisor.Run(ctx); err != nil && ctx.Err() == nil {
			fmt.Println("supervisor stopped:", err)
		}
	}()

	_ = counter.Increment(ctx, 10)
	_ = counter.Increment(ctx, 32)

	count, err := counter.Get(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println("count:", count)

	cancel()
	supervisor.Wait()
}
```

## Actors

An actor implements the `Actor` interface:

```go
type Actor interface {
	ID() string
	Run(context.Context) error
	Inspect() sup.Spec
}
```

Embed `*sup.BaseActor` when you only need a stable id and a default `Inspect` implementation:

```go
type Worker struct {
	*sup.BaseActor
}

func NewWorker(id string) *Worker {
	return &Worker{BaseActor: sup.NewBaseActor(id)}
}
```

For stateless actors, use `ActorFunc`:

```go
worker := sup.ActorFunc("health", func(ctx context.Context) error {
	<-ctx.Done()
	return nil
})
```

## Supervisors

Supervisors run actors and restart them according to a restart policy.

```go
supervisor := sup.NewSupervisor("root").
	Policy(sup.Transient).
	RestartDelay(500 * time.Millisecond).
	RestartLimit(3, time.Minute).
	Actors(actorA, actorB)

if err := supervisor.Run(ctx); err != nil {
	// Run returns ctx.Err() when ctx is canceled, or a terminal supervisor error.
}
```

### Restart policies

| Policy | Clean exit (`nil`) | Error or panic |
|---|---:|---:|
| `Permanent` | restart | restart |
| `Transient` | stop | restart |
| `Temporary` | stop | stop |

### Dynamic spawning

Use `Spawn` to start an actor after the supervisor already exists:

```go
supervisor := sup.NewSupervisor("jobs").Policy(sup.Temporary)
go supervisor.Run(ctx)

for _, job := range jobs {
	supervisor.Spawn(ctx, newJobActor(job))
}

supervisor.Wait()
```

### Observers

`SupervisorObserver` receives asynchronous lifecycle callbacks. Parent supervisor observers are inherited by child supervisors.

```go
observer := &sup.SupervisorObserver{
	OnActorRegistered: func(s *sup.Supervisor, a sup.Actor) {
		fmt.Println("registered", a.ID())
	},
	OnActorStarted: func(s *sup.Supervisor, a sup.Actor) {
		fmt.Println("started", a.ID())
	},
	OnActorStopped: func(s *sup.Supervisor, a sup.Actor, err error) {
		fmt.Println("stopped", a.ID(), err)
	},
	OnActorRestarting: func(s *sup.Supervisor, a sup.Actor, count int, lastErr error) {
		fmt.Println("restarting", a.ID(), count, lastErr)
	},
	OnSupervisorTerminal: func(s *sup.Supervisor, err error) {
		fmt.Println("terminal", s.ID(), err)
	},
}

root := sup.NewSupervisor("root").Observer(observer).Actor(worker)
```

## Typed inboxes

`CastInbox[T]` is for asynchronous messages. `CallInbox[T, R]` is for request/reply interactions.

### Cast inbox

```go
inbox := sup.NewCastInbox[IncrementMessage](8)

err := inbox.Cast(ctx, IncrementMessage{Amount: 1})    // blocks until queued or ctx is done
err = inbox.TryCast(ctx, IncrementMessage{Amount: 1}) // returns ErrCastInboxFull if full

for msg := range inbox.Receive() {
	// process msg
}
```

### Call inbox

```go
inbox := sup.NewCallInbox[GetMessage, int](8)

value, err := inbox.Call(ctx, GetMessage{})

for req := range inbox.Receive() {
	req.Reply(42, nil)
}
```

Both inboxes expose `Close`, `Closed`, `Len`, and `Cap`.

## Signal

A `Signal[V]` stores a value, publishes updates, and can run one or more sources.

```go
count := sup.NewSignal("count", 0).
	InitialNotify().
	Equal(func(a, b int) bool { return a == b })

values := count.Subscribe(ctx)
updates := count.Watch(ctx)

go func() {
	for value := range values {
		fmt.Println(value)
	}
}()

_ = count.Write(ctx, 1)
```

Signals are actors. If a signal has sources, run it under a supervisor or in a goroutine:

```go
random := sup.NewSignal("random", 0).
	Poll(200*time.Millisecond, func(ctx context.Context) (int, error) {
		return rand.IntN(100), nil
	}).
	Throttle(time.Second)

root := sup.NewSupervisor("signals").Actor(random)
go root.Run(ctx)
```

### Sources

A source produces values for a signal:

- `Poll(interval, fn)` calls `fn` on each interval and emits the result.
- `FromChannel(ch)` emits values received from a channel.
- `SourceFunc` adapts a function into a custom source.

```go
ch := make(chan string)
status := sup.NewSignal("status", "offline").Source(sup.FromChannel(ch))
```

### Processors

Processors transform, filter, or delay values before they are stored and broadcast:

```go
processed := sup.NewSignal("processed", 0).
	Map(func(v int) int { return v * 2 }).
	Filter(func(v int) bool { return v >= 10 }).
	Debounce(100 * time.Millisecond)
```

Built-in processors:

- `Map(fn)` transforms each value.
- `Filter(fn)` drops values that do not match.
- `Debounce(wait)` emits the latest value after a quiet period.
- `Throttle(interval)` emits at most one value per interval.

### Derived

`Derived[V]` computes a read-only signal from one or more watcher signals.

```go
count := sup.NewSignal("count", 0)
doubled := sup.NewDerived("doubled", func() int {
	return count.Read() * 2
}, count).
	InitialNotify()

root := sup.NewSupervisor("root").Actors(count, doubled)
go root.Run(ctx)
```

`BatchWindow` controls how dependency updates are coalesced before recomputing.

### Effect

`Effect[V]` runs side effects for values emitted by a signal or derived signal.

```go
effect := count.Effect("log_count", func(ctx context.Context, value int) error {
	fmt.Println("count changed:", value)
	return nil
})

root := sup.NewSupervisor("root").Actors(count, effect)
```

## Controls

Controls expose typed inboxes for dynamic dispatch, such as from the `hub` HTTP API.

```go
func (c *Counter) Controls() []sup.Control {
	return []sup.Control{
		sup.NewCastControl("increment", c.IncrementInbox),
		sup.NewCallControl("get", c.GetInbox),
	}
}
```

`NewCastControl[T]` decodes JSON input and dispatches to a `CastInbox[T]`.
`NewCallControl[T, R]` decodes JSON input, dispatches to a `CallInbox[T, R]`, and returns the reply.

Input schemas are inferred from Go types and JSON tags.

## Hub

The `github.com/webermarci/sup/hub` package exposes actors, controls, signals, and events over HTTP. It also serves the embedded debug UI at `/debug`.

```go
registry := hub.New("registry",
	hub.WithActor(counter),
	hub.WithSignal(counter.StateSignal),
)

root := sup.NewSupervisor("root").
	Observer(registry.Observer()).
	Actors(counter, counter.StateSignal, registry)

go root.Run(ctx)
go http.ListenAndServe(":8080", registry.Handler())
```

Endpoints include:

- `GET /actors`
- `GET /actors/{actorID}`
- `GET /actors/{actorID}/controls`
- `POST /actors/{actorID}/controls/{controlName}`
- `GET /signals`
- `GET /signals/{signalID}`
- `GET /events`
- `GET /events/stream`
- `GET /debug`

## Packages

- `sup` — Core actors, supervisors, typed inboxes, controls, signals, sources, processors, derived signals, and effects.
- `sup/hub` — HTTP API and debug UI for actors, controls, signals, and events.
- `sup/exec` — Actor wrapper around `os/exec` commands.
- `sup/mesh` — NATS-backed actor for subscriptions.
- `sup/modbus` — Modbus actor for TCP/RTU/ASCII clients.
- `sup/mqtt` — MQTT actor for publish/subscribe clients.
- `sup/sse` — Server-Sent Events client actor.
- `sup/ws` — WebSocket client actor.
