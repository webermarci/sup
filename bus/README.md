# sup/bus

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/bus.svg)](https://pkg.go.dev/github.com/webermarci/sup/bus)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`bus` provides reactive value propagation for Go programs. It is built on top of [`sup`](https://github.com/webermarci/sup) actors and designed for systems that poll hardware, sensors, or external services, and need to broadcast changes to multiple consumers.

## Installation

```bash
go get github.com/webermarci/sup/bus
```

## Concepts

| Type | Direction | Use case |
|---|---|---|
| `Signal` | Read → broadcast | Poll a register, sensor, or API; notify subscribers on change |
| `Target` | Write → hardware | Accept writes from callers; forward to a handler on success |

Both types are actors. They do nothing until `Run(ctx)` is called.

## Signal

A `Signal` periodically calls a poll function and broadcasts the result to all current subscribers whenever the value changes.

```go
signal := bus.NewSignal(func() (uint16, error) {
    return modbusClient.ReadRegister(0x01)
}).
    WithInterval(100 * time.Millisecond).
    WithInitialValue(0).
    WithInitialNotify(true)

go signal.Run(ctx)

ch := signal.Subscribe(ctx)
for v := range ch {
    fmt.Printf("register 0x01 changed: %d\n", v)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithInterval(d)` | 1s | How often the poll function is called |
| `WithInitialValue(v)` | zero value | Value before the first successful poll |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |

### Behaviour

- If the poll function returns an error, the value is **not updated** and subscribers are **not notified**.
- Subscribers are notified only when the value **changes** — repeated identical results are silently dropped.
- Subscribing with a canceled context is a no-op; the returned channel is closed immediately.
- Canceling a subscriber's context closes its channel and removes it from the broadcast list.

## Target

A `Target` accepts writes via `SetValue`, calls an update function with the new value, and — on success — updates the stored value and notifies subscribers.

```go
target := bus.NewTarget(func(v uint16) error {
    return modbusClient.WriteRegister(0x02, v)
}).WithInitialValue(0)

go target.Run(ctx)

if err := target.SetValue(42); err != nil {
    log.Printf("write rejected: %v", err)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithInitialValue(v)` | zero value | Value before the first successful write |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |

### Behaviour

- `SetValue` is synchronous — it blocks until the update function has returned.
- If the update function returns an error, the stored value is **not updated** and subscribers are **not notified**. The error is returned to the caller.
- `Sync` re-runs the update function with the current stored value, useful for forcing hardware to match state after a reconnect.

## Full Example

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/webermarci/sup/bus"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Poll a temperature sensor every 500ms
    temp := bus.NewSignal(func() (float64, error) {
        return readTemperatureSensor()
    }).
        WithInterval(500 * time.Millisecond).
        WithInitialNotify(true)

    // Control a heater relay
    heater := bus.NewTarget(func(on bool) error {
        return setHeaterRelay(on)
    }).WithInitialValue(false)

    go temp.Run(ctx)
    go heater.Run(ctx)

    // Subscribe to temperature changes and control heater accordingly
    tempCh := temp.Subscribe(ctx)
    go func() {
        for t := range tempCh {
            if err := heater.SetValue(t < 20.0); err != nil {
                fmt.Printf("heater control failed: %v\n", err)
            }
        }
    }()

    time.Sleep(10 * time.Second)
}
```

## Using with a Supervisor

Both `Signal` and `Target` implement the `sup.Actor` interface via their `Run` method, so they can be placed directly under a supervisor:

```go
supervisor := sup.NewSupervisor(
    sup.WithActors(temp, heater),
    sup.WithPolicy(sup.Permanent),
    sup.WithRestartDelay(time.Second),
)

supervisor.Run(ctx)
```
