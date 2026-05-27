# sup/ui

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/ui.svg)](https://pkg.go.dev/github.com/webermarci/sup/ui)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/ui` provides a web-based dashboard to observe the internal state of your actors. The dashboard is intentionally read-only so that state mutation remains isolated inside the actors themselves and the UI only observes and displays state changes.

## Features

- **Real-Time Updates:** Uses Server-Sent Events (SSE) to push state changes to the browser in real time.
- **SvelteKit Frontend:** The UI is implemented with SvelteKit and Vite for a modern, component-driven frontend.
- **Embeddable Static Assets:** Built frontend assets are embedded into the Go binary using `embed.FS`. See `sup/ui/frontend/frontend.go` which contains `//go:embed all:build` and `//go:generate` directives.
- **Prebuilt:** A `build` directory is included in the repository so you can compile and run the Go binary without having Node installed.
- **Actor-Model Compliant:** The dashboard only observes and displays state; it does not mutate actor state.
- **Automatic Type Inference:** Values are automatically shown as `boolean`, `number`, `string`, or `json` for nice formatting.

## Installation

```bash
go get github.com/webermarci/sup/ui
```

## Usage

Integrating the dashboard into your `sup` application requires only a few lines. The dashboard is just another supervised actor and exposes an `http.Handler()` that serves both the API and the frontend.

```go
package main

import (
	"context"
	"net/http"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/control"
	"github.com/webermarci/sup/ui"
)

func main() {
	// Assume you have some actors that implement bus.Provider
	sensorActor := NewSensorActor("temperature")
	statusActor := NewStatusActor("status")

	registry := control.NewRegistry("registry",
		control.WithActor(sensorActor),
		control.WithActor(statusActor),
	)

	// Create a supervisor and add your actors
	supervisor := sup.NewSupervisor("root",
		sup.WithActor(sensorActor),
		sup.WithActor(statusActor),
		sup.WithActor(registry),
	)

	// Create the Dashboard actor and attach your providers
	dashboard := ui.NewDashboard(registry)

	// Start the HTTP server to serve the UI
	go func() {
		http.ListenAndServe(":8080", dashboard.Handler())
	}()

	// Run the supervisor tree
	supervisor.Run(context.Background())
}
```

With the repository's prebuilt `sup/ui/frontend/build` present, the dashboard will serve the static site embedded in the binary.
