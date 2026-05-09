package main

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/ui"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	input := sup.NewPeriodicSignal("input", func(ctx context.Context) (int, error) {
		return rand.IntN(255), nil
	}, 100*time.Millisecond)

	throttledInput := sup.NewThrottledSignal("throttled_input", input, 2*time.Second)

	throttledInputASCII := sup.NewComputedSignal("throttled_input_ascii", func() string {
		return string(byte(throttledInput.Read()))
	}, throttledInput)

	throttledInputEven := sup.NewComputedSignal("throttled_input_even", func() bool {
		return throttledInput.Read()%2 == 0
	}, throttledInput)

	counter := sup.NewPushedSignal("counter", func(ctx context.Context, i int) error {
		return nil
	})

	combined := sup.NewComputedSignal("combined", func() struct {
		Input int
		ASCII string
		Even  bool
	} {
		return struct {
			Input int
			ASCII string
			Even  bool
		}{
			Input: throttledInput.Read(),
			ASCII: throttledInputASCII.Read(),
			Even:  throttledInputEven.Read(),
		}
	}, throttledInput, throttledInputASCII, throttledInputEven)

	dashboard := ui.NewDashboard("dashboard",
		ui.WithObserve(input),
		ui.WithObserve(throttledInput),
		ui.WithObserve(throttledInputASCII),
		ui.WithObserve(throttledInputEven),
		ui.WithObserve(counter),
		ui.WithObserve(combined),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(
			dashboard,
			input,
			throttledInput,
			throttledInputASCII,
			throttledInputEven,
			counter,
			combined,
		),
		sup.WithLogger(slog.Default()),
	)

	go supervisor.Run(ctx)
	go http.ListenAndServe(":8080", dashboard.Handler())

	i := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.NewTicker(time.Second).C:
			counter.Write(ctx, i)
			i++
		}
	}
}
