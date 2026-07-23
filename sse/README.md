# sup/sse

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/sse.svg)](https://pkg.go.dev/github.com/webermarci/sup/sse)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/sse` is a supervised Server-Sent Events client built on top of the [sup](https://github.com/webermarci/sup) actor library.

## Features

- **Supervised lifecycle:** Connection and stream failures return an error so the supervisor can reconnect.
- **Last-Event-ID:** The actor tracks the last event ID and sends it when reconnecting.
- **Stream timeout:** A configurable timeout detects streams that stop producing data.

## Installation

```bash
go get github.com/webermarci/sup/sse
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/sse"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := func(event sse.Event) {
		fmt.Println(event)
	}

	actor := sse.NewActor("actor", "http://localhost:8080", handler,
		sse.WithTimeout(10*time.Second),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	if err := supervisor.Run(ctx); err != nil {
		fmt.Println("supervisor failed:", err)
	}
}
```

Cancellation closes the stream and makes `Run` return `nil`. Use
`WithOnConnect`, `WithOnError`, `WithLastEventID`, or `WithHTTPClient` when the
corresponding lifecycle hook or transport customization is needed.
