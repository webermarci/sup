// Package rx provides shared reactive state and composable channel helpers.
//
// Signal is passive, writable state with independent, coalescing
// subscriptions. Derived is a supervised, read-only value computed from
// multiple dependencies. Map, Filter, Distinct, Debounce, and throttle helpers
// transform subscriptions using ordinary Go channels.
//
// Helpers close their output when their input closes. A subscription context
// therefore provides cancellation for an entire nested helper pipeline.
//
// It is a signal-based reactive layer, not an implementation of ReactiveX.
package rx
