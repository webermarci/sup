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

type Data struct {
	Time   string
	Number int
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	dateTime := sup.NewPushedSignal("date_time", func(ctx context.Context, s string) error {
		return nil
	})

	randomNumber := sup.NewPolledSignal("random_number", func(context.Context) (int, error) {
		return rand.IntN(100), nil
	}, 500*time.Millisecond)

	isEven := sup.NewComputedSignal("is_even", func() bool {
		return randomNumber.Read()%2 == 0
	}, randomNumber)

	jsonData := sup.NewComputedSignal("json_data", func() Data {
		return Data{
			Time:   dateTime.Read(),
			Number: randomNumber.Read(),
		}
	}, dateTime, randomNumber)

	dashboard := ui.NewDashboard("dashboard",
		ui.WithObserve(dateTime),
		ui.WithObserve(randomNumber),
		ui.WithObserve(isEven),
		ui.WithObserve(jsonData),
	)

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(
			dashboard,
			dateTime,
			randomNumber,
			isEven,
			jsonData,
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
			dateTime.Write(ctx, time.Now().Format(time.RFC3339))
			i++
		}
	}
}
