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
| `Mirror` | Read (Lazy) | Transform or combine existing values without extra goroutines |
| `Trigger` | Write → hardware | Accept writes from callers; forward to a handler on success |

Both types are actors. They do nothing until `Run(ctx)` is called.

## Signal

A `Signal` periodically calls a poll function and broadcasts the result to all current subscribers whenever the value changes.

```go
signal := bus.NewSignal("signal", func() (uint16, error) {
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

## Mirror

A `Mirror` provides a lazy, functional transformation of one or more `Readables`. It does not require a goroutine or a mailbox; it calculates its value on-demand when `Read()` is called.

```go
tempC := bus.NewSignal(...)

// Simple transformation
tempF := bus.NewMirror(func() float64 {
    return tempC.Read()*9/5 + 32
})

// Complex aggregation
isSafe := bus.NewMirror(func() bool {
    // Capture multiple signals in a closure for type-safe logic
    return tempC.Read() < 100.0 && pressure.Read() < 10.5
})
```

## Trigger

A `Trigger` accepts writes via `Write`, calls an update function with the new value, and — on success — updates the stored value and notifies subscribers.

```go
trigger := bus.NewTrigger("trigger", func(v uint16) error {
    return modbusClient.WriteRegister(0x02, v)
}).WithInitialValue(0)

go trigger.Run(ctx)

if err := trigger.Write(42); err != nil {
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
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Inputs
	temp := bus.NewSignal("temperature", func() (float64, error) {
		return readTemperatureSensor()
	}).WithInterval(500 * time.Millisecond)

	// 2. Logic (Mirror)
	// Automatically determine if heating is needed
	needsHeat := bus.NewMirror(func() bool {
		return temp.Read() < 20.0
	})

	// 3. Output
	heater := bus.NewTrigger("heater", func(on bool) error {
		return setHeaterRelay(on)
	})

	go temp.Run(ctx)
	go heater.Run(ctx)

	// Using the Mirror in a control loop
	tempCh := temp.Subscribe(ctx)
	go func() {
		for range tempCh {
			// Read the logic from the mirror and write to the trigger
			if err := heater.Write(needsHeat.Read()); err != nil {
				fmt.Printf("heater control failed: %v\n", err)
			}
		}
	}()

	time.Sleep(10 * time.Second)
}
```

## Using with a Supervisor

Both `Signal` and `Trigger` implement the `sup.Actor` interface via their `Run` method, so they can be placed directly under a supervisor. `Mirror` is passive and does not need to be supervised.

```go
supervisor := sup.NewSupervisor("root",
	sup.WithActors(temp, heater),
	sup.WithPolicy(sup.Permanent),
	sup.WithRestartDelay(time.Second),
)

supervisor.Run(ctx)
```
