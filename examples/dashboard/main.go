package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/httpserver"
	"github.com/webermarci/sup/hub"
	"github.com/webermarci/sup/rx"
)

const (
	controlInterval = 500 * time.Millisecond
	temperatureStep = 0.25
)

// The controller moves the measured temperature toward the setpoint. The hub
// can change enabled and setpoint; measured is read-only telemetry.
type temperatureController struct {
	enabled     *rx.Signal[bool]
	setpoint    *rx.Signal[float64]
	measurement *rx.Signal[float64]
}

func newTemperatureController() *temperatureController {
	return &temperatureController{
		enabled:     rx.NewSignal("enabled", false),
		setpoint:    rx.NewSignal("temperature_setpoint_celsius", 22.0),
		measurement: rx.NewSignal("temperature_celsius", 20.0),
	}
}

func (c *temperatureController) ID() string {
	return "temperature_controller"
}

func (c *temperatureController) Run(ctx context.Context) error {
	ticker := time.NewTicker(controlInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !c.enabled.Value() {
				continue
			}

			current := c.measurement.Value()
			target := c.setpoint.Value()
			next := current

			switch {
			case current < target:
				next = min(current+temperatureStep, target)
			case current > target:
				next = max(current-temperatureStep, target)
			}

			if next != current {
				c.measurement.Set(next)
			}
		}
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	controller := newTemperatureController()
	status := rx.NewDerived("controller_status", func() string {
		if !controller.enabled.Value() {
			return "off"
		}

		current := controller.measurement.Value()
		target := controller.setpoint.Value()
		switch {
		case current < target:
			return "heating"
		case current > target:
			return "cooling"
		default:
			return "at setpoint"
		}
	}, controller.enabled, controller.setpoint, controller.measurement)

	dashboard := hub.New("dashboard").
		RegisterActors(controller).
		RegisterWritableSignal(controller.enabled).
		RegisterWritableSignal(controller.setpoint).
		RegisterSignal(controller.measurement).
		RegisterSignal(status)

	server := httpserver.NewActor("dashboard.http",
		func(context.Context) (*http.Server, error) {
			return &http.Server{
				Addr:    "127.0.0.1:8080",
				Handler: dashboard,
			}, nil
		},
	)

	root := sup.NewSupervisor("root", sup.Transient).
		AddActors(controller, status, dashboard, server)

	fmt.Println("temperature dashboard: http://localhost:8080")
	if err := root.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "dashboard stopped:", err)
	}
}
