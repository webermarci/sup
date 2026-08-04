// Package rx provides shared reactive state and composable channel helpers.
//
// # Getting started
//
// Create a passive Signal for shared state and a Derived value for computed
// state:
//
//	status := rx.NewSignal("status", "starting")
//	summary := rx.NewDerived("summary", func() string {
//		return "service: " + status.Value()
//	}, status)
//
// A Derived value is an actor, so add it to a supervisor when it should keep
// recomputing:
//
//	supervisor := sup.NewSupervisor("root", sup.WithActors(summary))
//	go supervisor.Run(ctx)
//
// Subscribe to values or change notifications from any goroutine:
//
//	values := summary.Subscribe(ctx)
//	status.Set("ready")
//	value := <-values
//
// Map, Filter, Distinct, Debounce, and throttle helpers transform ordinary Go
// channels. They can be nested into pipelines and do not require a separate
// pipeline supervisor.
//
// # Semantics
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
