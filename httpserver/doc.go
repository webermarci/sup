// Package httpserver adapts standard-library net/http servers to sup actors.
//
// # Getting started
//
// Create an HTTP server actor and place it under a sup Supervisor:
//
//	server := httpserver.NewActor("http", func(context.Context) (*http.Server, error) {
//		return &http.Server{
//			Addr:    ":8080",
//			Handler: handler,
//		}, nil
//	})
//
//	supervisor := sup.NewSupervisor("root", sup.Transient).AddActors(server)
//
// The server factory is called once for every execution attempt and must return a
// fresh *http.Server because a server cannot be reused after Shutdown. By
// default, the actor calls ListenAndServe. Use SetServeFunc for TLS, custom
// listeners, or another serving mode. Use SetShutdownTimeout to bound the
// graceful shutdown period. Configure these methods before the first Run call.
//
// Context cancellation performs graceful shutdown with the configured timeout.
// http.ErrServerClosed is treated as a clean stop. Startup and unexpected
// serving errors are returned to the supervisor and handled by its restart
// policy.
//
// See the package README and the package reference at
// https://pkg.go.dev/github.com/webermarci/sup/httpserver for the complete API.
package httpserver
