// Package hub exposes selected sup actors and reactive signals over HTTP.
//
// The hub provides current actor and signal snapshots, a supervision graph
// projected from runtime registration events, bounded recent event history,
// and a live server-sent event stream. Writable signals may be explicitly
// exposed for outside interaction; all other registered signals are read-only.
package hub
