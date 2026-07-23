# sup/modbus

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/modbus.svg)](https://pkg.go.dev/github.com/webermarci/sup/modbus)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/modbus` is a high-reliability Modbus client implementation for Go, built on top of the [sup](https://github.com/webermarci/sup) actor library. It provides a thread-safe, supervised, and observable way to interact with Modbus devices over TCP, RTU, or ASCII.

## Why this exists?

Modbus is a single-threaded protocol by nature. If two different parts of your application try to access the same Serial or TCP port simultaneously, you get collisions, corrupted data, or "resource busy" errors. 

This library solves that by treating the Modbus connection as an **Actor**. All requests are queued in a mailbox and processed sequentially by the actor loop, ensuring hardware access is perfectly serialized.

## Features

- **Actor-based concurrency:** Concurrent callers are serialized through a typed inbox.
- **Supervised lifecycle:** Fatal connection errors return from `Run` so the supervisor can reconnect.
- **Protocol support:** TCP, RTU, and ASCII transports.
- **Industrial timing:** RS485 RTS delays and custom inter-frame quiet time.

## Installation

```bash
go get github.com/webermarci/sup/modbus
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/modbus"
)

func main() {
	client := modbus.NewActor(
		"controller",
		modbus.TCP,
		"192.168.1.50:502",
		1,
		modbus.WithTimeout(2*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(client),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(time.Second),
	)

	go supervisor.Run(ctx)

	res, err := client.ReadHoldingRegisters(100, 2)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Register Data: %X\n", res)
}
```

Calls made while the actor is starting wait in its inbox until the connection is
ready or the configured timeout expires. Cancellation is a clean shutdown and
makes `Run` return `nil`.

## Reactive signal integration

Use an actor to poll registers and publish them through a signal:

The signal types are provided by `github.com/webermarci/sup/rx`.

```go
registers := rx.NewSignal("holding-registers", []byte(nil))
poller := sup.ActorFunc("holding-registers.poller", func(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			value, err := client.ReadHoldingRegisters(100, 2)
			if err != nil {
				return err
			}
			registers.Set(value)
		case <-ctx.Done():
			return nil
		}
	}
})

supervisor := sup.NewSupervisor("root",
	sup.WithActors(client, poller),
	sup.WithPolicy(sup.Transient),
	sup.WithRestartDelay(time.Second),
)
```
