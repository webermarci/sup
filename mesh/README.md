# sup/mesh

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/mesh.svg)](https://pkg.go.dev/github.com/webermarci/sup/mesh)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/mesh` provides a small set of NATS-backed actors built on top of the `sup` actor library. The package exposes a single `Actor` type that manages its own NATS connection and can both publish and subscribe to subjects. The actor's responsibility is connection lifecycle and supervision — subscription handlers are executed directly by the NATS client and should manage their own concurrency if needed.

## Installation

```bash
go get github.com/webermarci/sup/mesh
```

## Quick Start

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

	options := nats.GetDefaultOptions()
	options.Url = "nats://localhost:4222"
	
	actor := mesh.NewActor("node", options,
		mesh.WithSubscription("devices.updates", func(b []byte) { 
			fmt.Println("received:", string(b)) 
		}),
		mesh.WithQueueSubscription("devices.updates", "worker-group", func(b []byte) { 
      fmt.Println("received in worker group:", string(b)) 
    }),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	go supervisor.Run(ctx)

	// Give the connection a moment to establish
	time.Sleep(250 * time.Millisecond)

	if err := actor.Publish("topic", []byte("hello mesh")); err != nil {
		fmt.Println("publish error:", err)
	}
}
```
