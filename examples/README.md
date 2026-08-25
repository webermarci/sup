# Examples

Each example is a standalone program that can be run from the repository root.

- `actor` introduces actors, supervisors, and typed inboxes.
- `signal` manages a temperature setpoint with subscriptions, `Set`, and atomic `Update`.
- `pipeline` turns signal values into temperature alarms with `Filter` and `Map`.
- `transitions` observes changes from a motor-state signal.
- `derived` computes Fahrenheit from a Celsius signal. Add the derived value to a supervisor when it should keep recomputing as its dependency changes.
- `dashboard` combines actors, signals, derived state, the hub HTTP API, and a supervised HTTP server.

```bash
go run ./examples/actor
go run ./examples/signal
go run ./examples/pipeline
go run ./examples/transitions
go run ./examples/derived
go run ./examples/dashboard
```
