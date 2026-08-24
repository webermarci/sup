// Package hub exposes selected sup actors and reactive signals over HTTP.
//
// # Getting started
//
// Create a Hub, explicitly register the actors and signals that should be
// visible, and add the hub to the same supervisor tree as the sources:
//
//	h := hub.New("debug",
//		hub.WithActor(worker),
//		hub.WithSignal(status),
//	)
//
//	root := sup.NewSupervisor("root", sup.Transient).
//		AddEventSink(h).
//		AddActors(worker, h)
//
//	go root.Run(ctx)
//	http.ListenAndServe(":8080", h.Handler())
//
// The hub provides current actor and signal snapshots, a supervision graph
// projected from runtime registration events, bounded recent event history,
// and a live server-sent event stream. Only explicitly registered actors and
// signals are exposed.
//
// WithSignal is read-only. WithWritableSignal additionally enables PATCH
// requests to change the signal, so use it only for values that are safe to
// control externally. The package does not provide authentication or
// authorization; protect the Handler with the application's own middleware or
// network boundary when it is not confined to a trusted environment.
//
// The Handler is prefix-agnostic and can be mounted below a path with
// http.StripPrefix. See the repository README and the package reference at
// https://pkg.go.dev/github.com/webermarci/sup/hub for endpoints and payloads.
package hub
