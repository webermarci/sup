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
| `Signal` | Read → broadcast | Poll a register, sensor, or API; notify subscribers on update |
| `Computed` | Notify → Update | Eagerly update value when dependencies change; broadcast updates |
| `Debounce` | Notify → Wait → Broadcast | Ignore rapid updates until the source is quiet; prevent noise |
| `Throttle` | Notify → Limit → Broadcast | Limit the maximum rate of updates to prevent overwhelming consumers |
| `Link` | Subscribe → Write | Connect a `Provider` to a `Writer` as a supervised actor |
| `ViewFunc` | Read (Lazy) | Transform or combine existing values without any goroutines |
| `Trigger` | Write → update | Accept writes from callers; forward to a handler on success |

Active types (`Signal`, `Computed`, `Debounce`, `Throttle`, `Link`, `Trigger`) are actors and should be managed with a supervisor.

## Signal

A `Signal` periodically calls an update function and broadcasts the result to all current subscribers.

```go
signal := bus.NewSignal("signal", func(ctx context.Context) (uint16, error) {
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
| `WithInterval(d)` | 1s | How often the update function is called |
| `WithInitialValue(v)` | zero value | Value before the first successful update |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |
| `WithEqual(func)` | nil | Custom equality function used to suppress identical updates |

### Behaviour

- If the update function returns an error, the value is **not updated** and subscribers are **not notified**.
- If `WithEqual` is configured, repeated equal values are dropped.
- If `WithEqual` is not configured, every successful update is broadcast.
- Subscribing with a canceled context is a no-op; the returned channel is closed immediately.
- Canceling a subscriber's context closes its channel and removes it from the broadcast list.

## ViewFunc

A `ViewFunc` provides a lazy, zero-overhead functional transformation of one or more `Readers`. It calculates its value on demand when `Read()` is called. It is an adapter type, not an actor, and requires no supervision.

```go
tempC := bus.NewSignal(...)

// Simple transformation
tempF := bus.ViewFunc[float64](func() float64 {
	return tempC.Read()*9/5 + 32
})

tempF.Read()

// Complex aggregation
isSafe := bus.ViewFunc[bool](func() bool {
	return tempC.Read() < 100.0 && pressure.Read() < 10.5
})

isSafe.Read()
```

## Computed

A `Computed` actor eagerly updates its value whenever its dependencies notify it of a change. Unlike a `ViewFunc`, which is lazy, a `Computed` actor maintains its own state and broadcasts changes to its own subscribers.

```go
temp := bus.NewSignal(...)
humidity := bus.NewSignal(...)

heatIndex := bus.NewComputed("heatIndex", func() float64 {
	return calculateHeatIndex(temp.Read(), humidity.Read())
}, temp, humidity).
	WithCoalesceWindow(5 * time.Millisecond)

go heatIndex.Run(ctx)

ch := heatIndex.Subscribe(ctx)
for v := range ch {
	fmt.Printf("new heat index: %.2f\n", v)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithCoalesceWindow(d)` | 5ms | Delay used to batch concurrent dependency updates |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |
| `WithEqual(func)` | nil | Custom equality function used to suppress identical updates |

### Behaviour

- It calls the update function once during creation to establish the initial value.
- It subscribes to all provided dependencies and re-runs the update function whenever any dependency notifies it.
- **Glitch-free:** it batches concurrent dependency notifications within a `coalesceWindow` so diamond graphs do not cause redundant recalculations or torn states.
- If `WithEqual` is configured, equal results are not broadcast.
- If `WithEqual` is not configured, every recomputation is broadcast.

## Debounce

A `Debounce` actor delays broadcasting updates from its upstream source until the source has stopped changing for a specified wait duration. This is ideal for taming noisy sensors or rapid user inputs.

```go
button := bus.NewTrigger(...)

cleanButton := bus.NewDebounce("clean-button", button, 300*time.Millisecond).
	WithMaxWait(1 * time.Second)

go cleanButton.Run(ctx)

ch := cleanButton.Subscribe(ctx)
for v := range ch {
	fmt.Println("debounced button state:", v)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithMaxWait(d)` | 0 | Force an update after this duration, preventing infinite starvation |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- Emits only after the source has been quiet for the `wait` duration.
- Evaluates the source immediately upon creation so `Read()` yields a valid initial value.

## Throttle

A `Throttle` actor limits the rate at which updates from its upstream source are broadcast. It ensures that updates are sent at most once per interval, which is useful for decoupling fast internal state changes from slower consumers such as WebSockets or UI rendering.

```go
fastComputed := bus.NewComputed(...)

uiState := bus.NewThrottle("ui-throttle", fastComputed, 200*time.Millisecond)

go uiState.Run(ctx)

ch := uiState.Subscribe(ctx)
for state := range ch {
	websocket.Send(state)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- If the throttle window is open, the first event is emitted **immediately**.
- If events arrive while the window is closed, it keeps the latest one and emits it when the interval elapses.
- Evaluates the source immediately upon creation so `Read()` yields a valid initial value.

## Trigger

A `Trigger` accepts writes via `Write`, calls an update function with the new value, and, on success, updates the stored value and notifies subscribers.

```go
trigger := bus.NewTrigger("trigger", func(ctx context.Context, v uint16) error {
	return modbusClient.WriteRegister(0x02, v)
}).WithInitialValue(0)

go trigger.Run(ctx)

if err := trigger.Write(42); err != nil {
	fmt.Printf("write rejected: %v\n", err)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithInitialValue(v)` | zero value | Value before the first successful write |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- `Write` is synchronous; it blocks until the update function returns.
- If the update function returns an error, the stored value is **not updated** and subscribers are **not notified**.

## Link

A `Link` actor connects a readable `Provider` to a destination `Writer` such as a `Trigger`. This lets you route data through your reactive pipeline without writing unsupervised goroutines.

```go
wiring := bus.NewLink("websocket-wiring", uiState, websocketTrigger)

go wiring.Run(ctx)
```

### Behaviour

- It subscribes to the source provider and forwards each received value to the destination writer.
- It exits when the context is canceled or the source subscription closes.
- Writes are synchronous, so backpressure naturally propagates through the link.

## Full Example

```go
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Inputs
	temp := bus.NewSignal("temperature", func(ctx context.Context) (float64, error) {
		return readTemperatureSensor()
	}).WithInterval(10 * time.Millisecond)

	// Filter out sensor noise with Debounce
	cleanTemp := bus.NewDebounce("clean-temp", temp, 200*time.Millisecond)

	// 2. Logic
	needsHeat := bus.ViewFunc[bool](func() bool {
		return cleanTemp.Read() < 20.0
	})

	// 3. Output Control
	safeHeaterCmd := bus.NewThrottle("safe-heater", needsHeat, time.Second)

	// 4. Output Actuator
	heater := bus.NewTrigger("heater", func(ctx context.Context, on bool) error {
		return setHeaterRelay(on)
	})

	// 5. Wiring
	wiring := bus.NewLink("heater-wiring", safeHeaterCmd, heater)

	// 6. Supervision
	supervisor := sup.NewSupervisor("root",
		sup.WithActors(temp, cleanTemp, safeHeaterCmd, heater, wiring),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	supervisor.Run(ctx)
}
```

## Using with a Supervisor

All active types (`Signal`, `Computed`, `Debounce`, `Throttle`, `Link`, and `Trigger`) implement the `sup.Actor` interface via their `Run` method, so they can be placed directly under a supervisor. `ViewFunc` is not supervised because it contains no running goroutines.

```go
supervisor := sup.NewSupervisor("root",
	sup.WithActors(temp, heater, cleanTemp, safeHeaterCmd, wiring),
	sup.WithPolicy(sup.Permanent),
	sup.WithRestartDelay(time.Second),
)

supervisor.Run(ctx)
```
