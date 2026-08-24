// Package sup provides a small actor supervision toolkit for Go.
//
// Actors own application behavior and mutable state. Supervisors own actor
// lifecycle, restart policy, and failure handling. Supervisors are actors too,
// so applications can build nested supervision trees from the same interface.
//
// # Getting started
//
// Define an Actor with an ID and a Run method, then add it to a Supervisor:
//
//	supervisor := sup.NewSupervisor("root", sup.Transient).AddActors(worker)
//
// Run the root supervisor with the application's context. Context cancellation
// is a clean shutdown and should make Run return nil. Return a non-nil error
// only when the actor has failed and the configured supervisor policy should
// decide whether to restart it.
//
// Each actor instance must belong to exactly one supervisor in a running tree.
// Complete all configuration before calling Run on the root supervisor.
// Supervisors invoke child Run methods and may invoke them again sequentially
// for restarts; the same actor instance must never run concurrently.
//
// NewSupervisor requires a restart policy and defaults to a one-second restart
// delay with no restart limit. Configure optional restart behavior with
// SetRestartDelay, SetRestartDelayFunc, and SetRestartLimit before the first
// Run call.
//
// # Communication
//
// Use CastInbox[T] for fire-and-forget messages and CallInbox[T, R] for
// request/reply operations. Inboxes are unbuffered, so Cast and Call block
// until the actor receives the message or the context is canceled. Call
// handlers should reply at most once and should tolerate a caller that has
// already canceled its context.
//
// # Reactive state
//
// The optional rx package provides passive Signal values and supervised
// Derived values. A Signal can be read and subscribed to from any goroutine;
// a Derived value actively watches dependencies and must be added to a
// supervisor tree if it is expected to update.
//
// # Runtime debugging
//
// Actors implementing EventHandler observe lifecycle events. The optional hub
// package builds an HTTP debugging surface from explicitly registered actors
// and signals.
//
// See the README for application-oriented examples and the package reference
// at https://pkg.go.dev/github.com/webermarci/sup for the complete API.
package sup
