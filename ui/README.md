# sup/ui

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/ui.svg)](https://pkg.go.dev/github.com/webermarci/sup/ui)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/ui` provides an out-of-the-box, beautifully designed web interface to observe the internal state of your actors. By strictly adhering to actor-model principles, the dashboard is **read-only**, ensuring that state mutation remains safely isolated within the actors themselves.

## Features

- **Real-Time Updates:** Uses Server-Sent Events (SSE) to instantly push state changes to the browser.
- **Zero Dependencies:** No npm, no external CSS frameworks, no frontend build steps. The HTML, CSS, and JS are highly optimized and embedded directly into your Go binary using `embed.FS`.
- **Actor-Model Compliant:** Purely observational. Actors broadcast their state, and the UI consumes it, preventing external UI mutations from violating actor concurrency guarantees.
- **Automatic Type Inference:** Automatically detects whether an actor's state is a `boolean`, `number`, `string`, or `json` and formats it accordingly.

## Installation

```bash
go get github.com/webermarci/sup/ui
```

## Usage

Integrating the dashboard into your `sup` application takes only a few lines of code.

### 1. Create the Dashboard
Initialize the dashboard and register the providers (actors) you want to observe using `ui.WithObserve()`.

```go
package main

import (
	"context"
	"net/http"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/ui"
)

func main() {
	// Assume you have some actors that implement bus.Provider
	sensorActor := NewSensorActor("temperature")
	statusActor := NewStatusActor("status")

	// Create the Dashboard actor and attach your providers
	dashboard := ui.NewDashboard("dashboard",
		ui.WithObserve(sensorActor),
		ui.WithObserve(statusActor),
	)

	// Create a supervisor and add your actors
	supervisor := sup.NewSupervisor("root",
		sup.WithActor(sensorActor),
		sup.WithActor(statusActor),
		sup.WithActor(dashboard), // The dashboard is just another supervised actor!
	)

	// Start the HTTP server to serve the UI
	go func() {
		http.ListenAndServe(":8080", dashboard.Handler())
	}()

	// Run the supervisor tree
	supervisor.Run(context.Background())
}
```
