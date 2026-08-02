# sup/sse

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/sse.svg)](https://pkg.go.dev/github.com/webermarci/sup/sse)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/sse` is a small adapter between an HTTP server-sent event stream and
`sup.Actor`. The caller constructs and executes an ordinary HTTP request; the
actor parses one returned stream per execution attempt.

## Usage

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/sse"
)

func main() {
	client := &http.Client{}

	events := sse.NewActor(
		"events",
		func(ctx context.Context, lastEventID string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"https://example.com/events",
				nil,
			)
			if err != nil {
				return nil, err
			}

			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Authorization", "Bearer "+os.Getenv("TOKEN"))
			if lastEventID != "" {
				req.Header.Set("Last-Event-ID", lastEventID)
			}

			return client.Do(req)
		},
		func(event sse.Event) {
			fmt.Printf("%s: %s\n", event.Type, event.Data)
		},
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(events),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(time.Second),
	)

	if err := supervisor.Run(context.Background()); err != nil {
		fmt.Println("supervisor failed:", err)
	}
}
```

## Contract

- The connection function is called once per execution attempt.
- It receives the latest parsed event ID and may send it in `Last-Event-ID`.
- It owns request construction, authentication, headers, cookies, redirects,
  proxies, TLS, timeouts, and HTTP client configuration.
- The actor closes every response body returned without an error.
- A `200 OK` response must use the `text/event-stream` content type and, when
  present, a UTF-8 charset.
- A `204 No Content` response is a clean stop and makes `Run` return `nil`.
- Context cancellation is also a clean actor shutdown.
- Event handlers run synchronously; blocking or panicking affects the current
  connection attempt.
- Stream lines use conventional LF or CRLF endings and are limited to 1 MiB.
- Connection, response, parsing, and unexpected-EOF errors are returned to the
  supervisor.
- Concurrent calls to `Run` return `sse.ErrActorRunning`.

Reconnection, backoff, logging, metrics, signals, and application state belong
to the supervisor or application rather than this package.
