# sup/sse

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/sse.svg)](https://pkg.go.dev/github.com/webermarci/sup/sse)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sse` provides a Server-Sent Events (SSE) client implemented as a `sup` actor. It is designed for consuming real-time event streams from web services and integrating them into your actor tree.

## Installation

```bash
go get github.com/webermarci/sup/sse
```

## Concepts

The `sse.Actor` connects to an HTTP endpoint and streams events. It provides:

- **Event Parsing** — Automatically parses `id`, `event` (name), and `data` fields.
- **Last-Event-ID Persistence** — Keeps track of the last received ID to support resumed connections.
- **Watchdog Timeout** — Detects hung connections if no data is received within a specified window.
- **Supervisor Integration** — Works seamlessly with `sup.Supervisor` for automatic reconnection and error handling.

## Usage

Create an actor by specifying the URL and a handler function for incoming events.

```go
handler := func(e sse.Event) {
    fmt.Printf("Received event %s: %s\n", e.Name, e.Data)
}

actor := sse.NewActor("https://api.example.com/events", handler,
    sse.WithTimeout(60 * time.Second),
)

// Manage it with a supervisor
supervisor := sup.NewSupervisor("sse-watcher",
    sup.WithActor(actor),
    sup.WithPolicy(sup.Permanent),
)

go supervisor.Run(ctx)
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithTimeout(d)` | 30s | Maximum time to wait between events before timing out |
| `WithOnConnect(f)` | nil | Callback invoked when the connection is established |
| `WithOnError(f)` | nil | Callback invoked when a connection or parsing error occurs |
| `WithHTTPClient(c)` | custom | Provide a custom `http.Client` for authentication or custom transport |

### Behaviour

- **Automatic Resumption:** The `Actor` stores the `lastID` internally. If the supervisor restarts the actor, it automatically includes the `Last-Event-ID` header in the next request.
- **Multiline Data:** SSE allows `data` fields to span multiple lines. The actor correctly buffers and joins these with newlines.
- **Clean Shutdown:** When the context is canceled, the actor closes the connection and returns `ctx.Err()`.
- **Watchdog:** If the server keeps the connection open but stops sending data (and no TCP keep-alive or heartbeats are present), the `WithTimeout` watchdog will close the connection, allowing the supervisor to restart it.

## Full Example

```go
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Define the event handler
	onEvent := func(e sse.Event) {
		log.Printf("[%s] %s: %s", e.ID, e.Name, e.Data)
	}

	// 2. Create the SSE actor
	client := sse.NewActor("http://localhost:8080/stream", onEvent,
		sse.WithTimeout(15*time.Second),
		sse.WithOnConnect(func(url, lastID string) {
			log.Printf("Connected to %s (Last ID: %s)", url, lastID)
		}),
	)

	// 3. Wrap it in a supervisor for resilience
	supervisor := sup.NewSupervisor("root",
		sup.WithActor(client),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(2*time.Second),
	)

	// 4. Run blocks until context is canceled
	if err := supervisor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("Supervisor failed: %v", err)
	}
}
```

## Using with a Supervisor

The `sse.Actor` implements the `sup.Actor` interface. Because it maintains the `Last-Event-ID` state in the struct, it is recommended to use the `sup.Permanent` policy. When the connection drops or times out, the supervisor will call `Run` again, and the actor will resume from where it left off.
