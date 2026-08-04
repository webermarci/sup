// Package httpserver adapts standard-library net/http servers to sup actors.
//
// A ServerFunc creates a fresh *http.Server for every execution attempt. This
// is important because an http.Server cannot be reused after Shutdown. By
// default, the actor calls ListenAndServe; WithServe supports TLS, custom
// listeners, and other serving modes.
//
// Context cancellation performs graceful shutdown with the configured timeout.
// http.ErrServerClosed is treated as a clean stop, while startup and unexpected
// serving errors are returned to the supervisor.
package httpserver
