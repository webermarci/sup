# sup/mesh

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/mesh.svg)](https://pkg.go.dev/github.com/webermarci/sup/mesh)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/mesh` provides lightweight NATS-backed messaging actors built on top of the `sup` actor library. It packages a supervised NATS client, a mailbox-backed `Publisher` (for serialized outbound publishes), and a simple `Subscriber` actor (for subject subscriptions and handler dispatch).

This package is useful when you want to integrate NATS into an actor tree and keep all networking and concurrency confined to supervised actors.

## Installation

```bash
go get github.com/webermarci/sup/mesh
```

## Concepts

| Type | Direction | Use case |
|---|---|---|
| `Client` | manages connection | Supervised NATS connection shared by publishers/subscribers |
| `Publisher` | Write → NATS | Serialize outbound publishes and surface publish errors to callers |
| `Subscriber` | NATS → Handler | Subscribe to a subject and call a user handler for each message |

Active types (`Client`, `Publisher`, `Subscriber`) are actors and should be supervised with `sup.Supervisor`.

## Client

The `Client` actor owns the `nats.Conn` and lifecycle of the underlying connection. Construct a client with `NewClient(name string, opts nats.Options)`.

- On `Run` the actor calls `opts.Connect()` and keeps the resulting `*nats.Conn` until the context is canceled.
- Use `Conn()` to obtain a thread-safe snapshot of the current connection (may be `nil` if not connected).
- When the context is canceled the client closes the connection and `Run` returns.

```go
opts := nats.Options{Servers: []string{nats.DefaultURL}}
client := mesh.NewClient("nats-client", opts)
```

## Publisher

`Publisher` serializes outbound publish requests using an internal `sup.Mailbox`. It exposes a synchronous `Publish([]byte) error` method that returns when the publish has completed (or failed).

Create a publisher with `NewPublisher(name string, client *Client, subject string, opts ...PublisherOption)`.

Options:

- `WithMailboxSize(n)` — change the default mailbox size (default is 16).

Behavior:

- `Publish` uses `sup.Call` to post a message to the publisher's mailbox and waits for the actor to perform the NATS publish and reply.
- If the NATS connection is not available or the client is not connected, `Publish` will return an error.

```go
pub := mesh.NewPublisher("publisher", client, "devices.updates")
if err := pub.Publish([]byte("hello world")); err != nil {
	fmt.Println("publish failed:", err)
}
```

## Subscriber

`Subscriber` is a thin supervised wrapper around `nats.Conn.Subscribe`. Construct with `NewSubscriber(name string, client *Client, subject string, handler func([]byte))`.

Behavior:

- On `Run` it checks `client.Conn()` and registers a subscription with the provided `handler` (the handler is called on the NATS client's goroutine).
- If the client is not initialized when `Run` starts, `Run` returns an error.
- When the context is canceled the actor unsubscribes and returns.

```go
sub := mesh.NewSubscriber("subscriber", client, "devices.updates", func(b []byte) {
	fmt.Println("got message:", string(b))
})
```

## Quick Start

A minimal example wiring a `Client`, a `Publisher`, and a `Subscriber` under a `sup.Supervisor`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/webermarci/sup"
	"github.com/webermarci/sup/mesh"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := nats.Options{Servers: []string{nats.DefaultURL}}
	client := mesh.NewClient("nats-client", opts)

	pub := mesh.NewPublisher("publisher", client, "devices.updates")
	sub := mesh.NewSubscriber("subscriber", client, "devices.updates", func(b []byte) {
		fmt.Println("received:", string(b))
	})

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(client, pub, sub),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	// Run the actor tree in the background so we can interact with it
	go supervisor.Run(ctx)

	// Give the client a moment to connect
	time.Sleep(250 * time.Millisecond)

	if err := pub.Publish([]byte("hello mesh")); err != nil {
		fmt.Println("publish error:", err)
	}
}
```
