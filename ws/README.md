# sup/ws

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/ws.svg)](https://pkg.go.dev/github.com/webermarci/sup/ws)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/ws` is a high-reliability WebSocket client implementation for Go, built on top of the [sup](https://github.com/webermarci/sup) actor library. It provides a thread-safe, supervised, and observable way to interact with WebSocket endpoints.

## Why this exists?

WebSocket connections are long-lived and inherently stateful. If two goroutines try to write to the same connection simultaneously, the result is a data race and a broken stream.

This library solves that by treating the WebSocket connection as an **Actor**. All outbound messages are queued in a mailbox and written sequentially by the actor loop, ensuring writes are perfectly serialized. Inbound messages are delivered to a handler function as they arrive.

## Features

- **Actor-based concurrency:** Thread-safe outbound writes through `Send`; the actor serializes them.
- **Supervised lifecycle:** Connection failures return an error so the supervisor can reconnect.
- **Binary and text support:** Messages retain their WebSocket frame type.
- **Keepalive pings:** A configurable ping interval helps detect silent connection failures.

## Installation

```bash
go get github.com/webermarci/sup/ws
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/ws"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(message ws.Message) {
		fmt.Println(message)
	}

	actor := ws.NewActor("actor", "wss://example.com/stream", handler,
		ws.WithPingInterval(15*time.Second),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	go supervisor.Run(ctx)

	_ = actor.Send(ws.MessageText, []byte(`{"action":"subscribe","channel":"updates"}`))
	_ = actor.Send(ws.MessageBinary, []byte{0x01, 0x02, 0x03})

	<-ctx.Done()
}
```

## Options

| Option | Default | Description |
|---|---:|---|
| `WithInboxSize(size)` | `64` | Capacity of the outbound message queue |
| `WithPingInterval(d)` | `15s` | Interval between WebSocket pings |
| `WithHTTPClient(client)` | default client | HTTP client used while dialing |
| `WithOnConnect(fn)` | nil | Called after a connection is established |
| `WithOnMessage(fn)` | nil | Called for every inbound message with inter-message duration |
| `WithOnError(fn)` | nil | Called for failures that make `Run` return an error |

Cancellation closes the connection normally and makes `Run` return `nil`.
