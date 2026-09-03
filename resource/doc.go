// Package resource provides supervised, serialized access to acquired
// resources.
//
// # Getting started
//
// An Actor acquires a resource for each execution attempt, receives operations
// directly, runs them one at a time, and releases the resource when the
// attempt ends. Acquisition failures and asynchronous [Actor.Cast] failures
// are returned to the supervisor. [Actor.Call] operation errors are also
// returned to their callers before the resource actor stops.
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
//	supervisor := sup.NewSupervisor("root", sup.Transient).AddActors(device)
//
// Use [Actor.SetReleaseTimeout] before the first [Actor.Run] call to bound the
// release context when the default timeout is not appropriate.
//
// Use [Actor.Call] when the caller needs a typed result:
//
//	value, err := device.Call(ctx, func(ctx context.Context, device Device) (int, error) {
//		return device.Read(ctx)
//	})
//
// Use [Actor.Cast] when the caller only needs direct handoff. A Cast operation
// error stops the resource actor so its supervisor can apply the restart
// policy. Both operations wait across acquisition attempts and supervisor
// restart delays until the actor receives their request, and stop waiting when
// their context is canceled. Requests that have already been received are
// never replayed on a new resource execution.
//
// A resource is released with a fresh timeout context when Run ends. That
// context preserves values from the actor context but is not canceled by the
// actor's shutdown cancellation.
package resource
