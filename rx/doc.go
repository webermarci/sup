// Package rx provides shared reactive state and composable channel helpers.
//
// # Choosing a reactive type
//
// Signal is passive, concurrency-safe, writable state with independent
// subscriptions. Derived is read-only state computed from one or more
// dependencies. Unlike Signal, Derived is an actor: its Run method must be
// managed by a supervisor if it should continue recomputing after construction.
//
// Map, Filter, Distinct, Debounce, and throttle helpers transform ordinary Go
// channels. They can be nested into pipelines and do not require a separate
// pipeline supervisor.
//
// # Important semantics
//
// Every Signal subscription starts with the current value. Subscriptions and
// helper outputs have capacity one and coalesce pending values to the latest
// value, so a slow subscriber does not block Set. Signal.Set is still a
// publication when the new value equals the old value.
//
// Subscribe and Watch channels close when their context is canceled. Both
// provide an initial notification: Subscribe sends the current value, while
// Watch sends one change notification. Helpers close their output when their
// input closes, so canceling the context used to create the first subscription
// cancels an entire nested helper pipeline. Debounce and throttle intervals
// must be positive.
//
// Derived recomputation is not transactional across independent dependencies.
// It may publish an intermediate result before converging on the latest state;
// values that must change atomically should be kept together in one Signal.
// Debounce and ThrottleLatest do not flush a pending value when their input
// closes.
//
// It is a signal-based reactive layer, not an implementation of ReactiveX.
//
// See the README for application examples and the package reference at
// https://pkg.go.dev/github.com/webermarci/sup/rx for the complete API.
package rx
