// Package resource provides supervised, serialized access to acquired
// resources.
//
// # Getting started
//
// An Actor acquires a resource for each execution attempt, receives operations
// directly, runs them one at a time, and releases the resource when the
// attempt ends. Acquisition failures and asynchronous Cast failures are
// returned to the supervisor. Call operation errors are returned to their
// callers and do not automatically restart the resource.
//
// The resource can be anything that should be owned and accessed serially: a
// serial port, a Modbus client, a network connection, or a device handle.
// The acquire function should create a fresh value and the release function
// should close or disconnect it:
//
//	device := resource.NewActor(
//		"serial",
//		func(ctx context.Context) (*Device, error) {
//			return openDevice(ctx)
//		},
//		func(ctx context.Context, device *Device) error {
//			return device.Close(ctx)
//		},
//	)
//
//	supervisor := sup.NewSupervisor("root", sup.Transient).AddActor(device)
//
// Use SetReleaseTimeout before the first Run call to bound the release
// context, when the default timeout is not appropriate.
//
// Use Call when the caller needs a typed result:
//
//	value, err := resource.Call(ctx, device, func(ctx context.Context, device Device) (int, error) {
//		return device.Read(ctx)
//	})
//
// Use Cast when the caller only needs direct handoff. A Cast operation error
// stops the resource actor so its supervisor can apply the restart policy.
// Both operations block until the actor receives their request and stop
// waiting when their context is canceled.
//
// A resource is released with a fresh timeout context when Run ends. That
// context preserves values from the actor context but is not canceled by the
// actor's shutdown cancellation.
package resource
