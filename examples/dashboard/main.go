package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/bus"
	"github.com/webermarci/sup/ui"
)

type Data struct {
	Text   string
	Number int
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	randomString := bus.NewSignal("random_string", func(context.Context) (string, error) {
		return rand.Text(), nil
	}).WithInterval(5 * time.Second)

	counter := bus.NewTrigger("counter", func(ctx context.Context, n int) error {
		return nil
	}).WithInitialValue(0)

	isEven := bus.NewComputed("is_even", func() bool {
		return counter.Read()%2 == 0
	}, counter)

	jsonData := bus.NewComputed("json_data", func() Data {
		return Data{
			Text:   randomString.Read(),
			Number: counter.Read(),
		}
	}, randomString, counter)

	dashboard := ui.NewDashboard("dashboard",
		ui.WithObserve(randomString),
		ui.WithObserve(counter),
		ui.WithObserve(isEven),
		ui.WithObserve(jsonData),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(dashboard, randomString, counter, isEven, jsonData),
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
