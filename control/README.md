# sup/control

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/control.svg)](https://pkg.go.dev/github.com/webermarci/sup/control)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/control` provides a lightweight HTTP control surface for `sup` applications. It exposes selected actors and signals through a registry so external tools, dashboards, CLIs, or automation can inspect state, subscribe to updates, and invoke explicitly registered control actions.

The package is intentionally opt-in: actors are only controllable when you register typed `Cast` or `Call` actions for them, and signals are exposed as read-only values with live update events.

## Features

- **HTTP Registry:** Serve a JSON index of exposed actors, signals, and their control metadata.
- **Typed Control Actions:** Register fire-and-forget `Cast` actions and request/reply `Call` actions with typed Go handlers.
- **Automatic JSON Schemas:** Input and output schemas are inferred from Go types and `json` struct tags.
- **Signal Exposure:** Expose `sup.ReadableSignal` values with inferred UI-friendly value types.
- **Live Updates:** Signal changes are streamed to clients with Server-Sent Events (SSE).
- **Actor-Model Friendly:** Control handlers can call an actor's public API while the actor keeps ownership of its internal state.
- **Supervisable:** `Registry` implements `sup.Actor`, so it can be managed by the same supervisor tree as the rest of the system.

## Installation

```bash
go get github.com/webermarci/sup/control
```

## Quick start

This example exposes a counter actor through HTTP. The control registry provides one cast action (`increment`), one call action (`get`), and a readable status signal.

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/control"
)

type incrementMsg struct{ Amount int }
type getCountMsg struct{}

type Counter struct {
	*sup.BaseActor
	casts *sup.CastInbox[incrementMsg]
	calls *sup.CallInbox[getCountMsg, int]
	count int
}

func NewCounter(id string) *Counter {
	return &Counter{
		BaseActor: sup.NewBaseActor(id),
		casts:     sup.NewCastInbox[incrementMsg](16),
		calls:     sup.NewCallInbox[getCountMsg, int](8),
	}
}

func (c *Counter) Increment(ctx context.Context, amount int) error {
	return c.casts.Cast(ctx, incrementMsg{Amount: amount})
}

func (c *Counter) Get(ctx context.Context) (int, error) {
	return c.calls.Call(ctx, getCountMsg{})
}

func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case msg, ok := <-c.casts.Receive():
			if !ok {
				return nil
			}
			c.count += msg.Amount

		case req, ok := <-c.calls.Receive():
			if !ok {
				return nil
			}
			req.Reply(c.count, nil)
		}
	}
}

type incrementInput struct {
	Amount int `json:"amount"`
}

type countOutput struct {
	Count int `json:"count"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	counter := NewCounter("counter")
	status := sup.NewPushedSignal("status", func(ctx context.Context, value string) error {
		return nil
	})

	registry := control.NewRegistry("control",
		control.WithActor(counter,
			control.Cast("increment", func(ctx context.Context, in incrementInput) error {
				return counter.Increment(ctx, in.Amount)
			}),
			control.Call("get", func(ctx context.Context, _ struct{}) (countOutput, error) {
				count, err := counter.Get(ctx)
				return countOutput{Count: count}, err
			}),
		),
		control.WithSignal(status),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(counter, status, registry),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	go func() {
		if err := http.ListenAndServe(":8080", registry.Handler()); err != nil {
			fmt.Println("control server stopped:", err)
		}
	}()

	go supervisor.Run(ctx)

	_ = status.Write(ctx, "online")

	<-ctx.Done()
	supervisor.Wait()
}
```

## Concepts

### Registry

`Registry` is the package's main actor. It stores the exposed actor and signal metadata, runs signal streams, and serves the HTTP API through `Handler()`.

```go
registry := control.NewRegistry("control",
	control.WithActor(actor),
	control.WithSignal(signal),
)

http.ListenAndServe(":8080", registry.Handler())
```

Because `Registry` implements `sup.Actor`, it should usually be supervised:

```go
supervisor := sup.NewSupervisor("root",
	sup.WithActors(appActor, registry),
	sup.WithPolicy(sup.Permanent),
)

supervisor.Run(ctx)
```

### Exposed actors

Use `WithActor` to include an actor in the registry index. By default this only exposes the actor's `ID` and `Inspect()` spec.

Add control actions with `Cast` and `Call`:

```go
registry := control.NewRegistry("control",
	control.WithActor(worker,
		control.Cast("reset", func(ctx context.Context, _ struct{}) error {
			return worker.Reset(ctx)
		}),
		control.Call("stats", func(ctx context.Context, _ struct{}) (Stats, error) {
			return worker.Stats(ctx)
		}),
	),
)
```

- `Cast[T]` registers a named command that accepts JSON input of type `T` and returns only success or error.
- `Call[T, R]` registers a named request that accepts JSON input of type `T` and returns a JSON response of type `R`.
- Empty HTTP request bodies are treated as `{}`, which is useful for `struct{}` inputs.
- Duplicate actor IDs, duplicate action names, or cast/call name collisions panic during registry construction.

### Exposed signals

Use `WithSignal` to expose any `sup.ReadableSignal[V]`:

```go
status := sup.NewPushedSignal("status", func(ctx context.Context, value string) error {
	return nil
})

registry := control.NewRegistry("control",
	control.WithSignal(status),
)
```

The registry index includes each signal's current value, `Inspect()` spec, and an inferred display type:

| Go type | Exposed type |
|---|---|
| `bool` | `boolean` |
| integer and float types | `number` |
| `string` | `string` |
| other values | `json` |

When a signal publishes a new value, the registry updates its snapshot and broadcasts a `signal:update` SSE event.

## HTTP API

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Returns all exposed actors and signals as JSON. |
| `GET` | `/events` | Opens an SSE stream for live registry events. |
| `POST` | `/actors/{actorID}/casts/{castName}` | Invokes a registered cast action. |
| `POST` | `/actors/{actorID}/calls/{callName}` | Invokes a registered call action and returns its JSON response. |

### Registry index

`GET /` returns a JSON object with `actors` and `signals` arrays:

```json
{
  "actors": [
    {
      "id": "counter",
      "spec": { "kind": "counter" },
      "control": {
        "casts": [
          {
            "name": "increment",
            "input_schema": {
              "type": "object",
              "properties": {
                "amount": { "type": "integer" }
              },
              "required": ["amount"]
            }
          }
        ],
        "calls": [
          {
            "name": "get",
            "input_schema": { "type": "object", "properties": {} },
            "output_schema": {
              "type": "object",
              "properties": {
                "count": { "type": "integer" }
              },
              "required": ["count"]
            }
          }
        ]
      }
    }
  ],
  "signals": [
    {
      "id": "status",
      "spec": { "kind": "pushed_signal" },
      "type": "string",
      "value": "online"
    }
  ]
}
```

### Cast actions

`POST /actors/{actorID}/casts/{castName}` decodes the request body into the cast input type and runs the registered handler.

Successful response:

```json
{ "ok": true }
```

### Call actions

`POST /actors/{actorID}/calls/{callName}` decodes the request body into the call input type and returns the handler's output as JSON.

Example response:

```json
{ "count": 5 }
```

### SSE events

`GET /events` returns an SSE stream. Signal updates are sent as `signal:update` events:

```text
event: signal:update
data: {"timestamp":1760000000000,"id":"status","value":"online"}
```

The stream also sends periodic `: ping` comments to keep long-lived connections open.

## JSON schema inference

`sup/control` derives lightweight JSON schemas from Go types:

- Primitive Go types map to JSON `boolean`, `string`, `integer`, or `number`.
- Slices and arrays become JSON arrays with an `items` schema.
- `map[string]T` becomes a JSON object with `additionalProperties`.
- Structs become JSON objects using exported fields and `json` tags.
- Fields tagged with `json:"-"` and unexported fields are ignored.
- Fields tagged with `omitempty` are omitted from the generated `required` list.

These schemas are intended for control clients and dashboards; validation is still performed by normal Go JSON decoding in the registered handler.
