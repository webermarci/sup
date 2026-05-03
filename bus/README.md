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
| `Computed` | Notify → Update | Eagerly update value when dependencies change; broadcast updates |
| `Debounce` | Notify → Wait → Broadcast | Ignore rapid updates until the source is quiet; prevent noise |
| `Throttle` | Notify → Limit → Broadcast | Limit the maximum rate of updates to prevent overwhelming consumers |
| `ViewFunc` | Read (Lazy) | Transform or combine existing values statically without any goroutines |
| `Trigger` | Write → update | Accept writes from callers; forward to a handler on success |

Active types (`Signal`, `Computed`, `Debounce`, `Throttle`, `Trigger`) are actors and should be managed with a supervisor.

## Signal

A `Signal` periodically calls a poll function and broadcasts the result to all current subscribers whenever the value changes.

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
| `WithInterval(d)` | 1s | How often the poll function is called |
| `WithInitialValue(v)` | zero value | Value before the first successful poll |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- If the poll function returns an error, the value is **not updated** and subscribers are **not notified**.
- Subscribers are notified only when the value **changes** — repeated identical results are silently dropped.
- Subscribing with a canceled context is a no-op; the returned channel is closed immediately.
- Canceling a subscriber's context closes its channel and removes it from the broadcast list.

## ViewFunc

A `ViewFunc` provides a lazy, zero-overhead functional transformation of one or more `Readers`. It calculates its value on-demand when `Read()` is called. It is an adapter type, not an actor, and requires no supervision.

```go
tempC := bus.NewSignal(...)

// Simple transformation
tempF := bus.ViewFunc[float64](func() float64 {
	return tempC.Read()*9/5 + 32
})

tempF.Read() // calculates fahrenheit from the latest celsius value

// Complex aggregation
isSafe := bus.ViewFunc[bool](func() bool {
	// Capture multiple signals in a closure for type-safe logic
	return tempC.Read() < 100.0 && pressure.Read() < 10.5
})

isSafe.Read() // calculates safety status from multiple signals
```

## Computed

A `Computed` actor eagerly updates its value whenever its dependencies notify it of a change. Unlike a `ViewFunc`, which is lazy, a `Computed` actor maintains its own state and broadcasts changes to its own subscribers.

```go
temp := bus.NewSignal(...)
humidity := bus.NewSignal(...)

// Eagerly compute heat index whenever temp or humidity changes
heatIndex := bus.NewComputed("heatIndex", func() float64 {
	return calculateHeatIndex(temp.Read(), humidity.Read())
}, temp, humidity)

// Coalesce window prevents glitches from rapid concurrent updates
heatIndex.WithCoalesceWindow(5 * time.Millisecond)

go heatIndex.Run(ctx)

// Subscribers receive updates automatically when dependencies change
channel := heatIndex.Subscribe(ctx)
for v := range channel {
	fmt.Printf("new heat index: %.2f\n", v)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithCoalesceWindow(d)` | 5ms | The delay used to batch concurrent dependency updates |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- It calls the update function once during creation to establish the initial value.
- It subscribes to all provided dependencies and re-runs the update function whenever any dependency notifies it.
- **Glitch-free:** It batches concurrent dependency notifications within a `coalesceWindow` (default 5ms) so complex diamond graphs don't cause multiple redundant recalculations or torn states.
- After each update, it broadcasts the new value to its subscribers.

## Debounce

A `Debounce` actor delays broadcasting updates from its upstream source until the source has stopped changing for a specified wait duration. This is ideal for taming noisy sensors or rapid user inputs.

```go
button := bus.NewTrigger(...)

// Ignore button presses until 300ms of quiet time passes
cleanButton := bus.NewDebounce("clean-button", button, 300*time.Millisecond).
	WithMaxWait(1 * time.Second) // Force a publish every 1s even if still noisy

go cleanButton.Run(ctx)

ch := cleanButton.Subscribe(ctx)
for v := range ch {
	fmt.Println("debounced button state:", v)
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithMaxWait(d)` | 0 | Forces an update after this duration, preventing infinite starvation |
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- Emits only after the source has been quiet for the `wait` duration.
- Evaluates the source immediately upon creation so `Read()` yields a valid initial value.

## Throttle

A `Throttle` actor limits the rate at which updates from its upstream source are broadcast. It ensures that updates are sent at most once per interval, making it ideal for decoupling fast internal state changes from slow consumers (like WebSockets or UI rendering).

```go
fastComputed := bus.NewComputed(...)

// Limit broadcasts to at most 5 times per second (200ms interval)
uiState := bus.NewThrottle("ui-throttle", fastComputed, 200*time.Millisecond)

go uiState.Run(ctx)

ch := uiState.Subscribe(ctx)
for state := range ch {
	// Safely send to a slow client without overwhelming it
	websocket.Send(state) 
}
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithInitialNotify(true)` | false | Send the current value immediately to each new subscriber |
| `WithSubscriberBuffer(n)` | 16 | The channel buffer size for subscribers |

### Behaviour

- If the throttle window is open, the first event is emitted **immediately** without artificial delay.
- If events arrive while the window is closed, it saves the absolute latest one and emits it exactly when the interval elapses (**trailing-edge** logic).
- Evaluates the source immediately upon creation so `Read()` yields a valid initial value.

## Trigger

A `Trigger` accepts writes via `Write`, calls an update function with the new value, and — on success — updates the stored value and notifies subscribers.

```go
trigger := bus.NewTrigger("trigger", func(ctx context.Context, v uint16) error {
	return modbusClient.WriteRegister(0x02, v)
}).WithInitialValue(0)

go trigger.Run(ctx)

if err := trigger.Write(ctx, 42); err != nil {
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

- `Write` is synchronous — it blocks until the update function has returned.
- If the update function returns an error, the stored value is **not updated** and subscribers are **not notified**. The error is returned to the caller.

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
	cleanTemp := bus.NewDebounce("clean-temp", temp, 200 * time.Millisecond)

	// 2. Logic (ViewFunc)
	// Automatically determine if heating is needed
	needsHeat := bus.ViewFunc[bool](func() bool {
		return cleanTemp.Read() < 20.0
	})

	// 3. Output Control (Throttle)
	// Don't spam the heater relay, limit commands to once per second max
	safeHeaterCmd := bus.NewThrottle("safe-heater", needsHeat, time.Second)

	// 4. Output Actuator
	heater := bus.NewTrigger("heater", func(ctx context.Context, on bool) error {
		return setHeaterRelay(on)
	})

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(temp, cleanTemp, safeHeaterCmd, heater),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(time.Second),
	)

	go supervisor.Run(ctx)

	// Using the Throttle in a control loop
	heaterCh := safeHeaterCmd.Subscribe(ctx)
	go func() {
		for cmd := range heaterCh {
			// Read the throttled command and write to the trigger
			if err := heater.Write(ctx, cmd); err != nil {
				fmt.Printf("heater control failed: %v\n", err)
			}
		}
	}()

	time.Sleep(10 * time.Second)
}
```

## Using with a Supervisor

All active types (`Signal`, `Computed`, `Debounce`, `Throttle`, and `Trigger`) implement the `sup.Actor` interface via their `Run` method, so they can be placed directly under a supervisor. Note that `ViewFunc` is not supervised as it contains no running goroutines.

```go
supervisor := sup.NewSupervisor("root",
	sup.WithActors(temp, heater, cleanTemp, safeHeaterCmd),
	sup.WithPolicy(sup.Permanent),
	sup.WithRestartDelay(time.Second),
)

supervisor.Run(ctx)
```
