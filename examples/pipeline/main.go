package main

import (
	"context"
	"fmt"

	"github.com/webermarci/sup/rx"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	temperature := rx.NewSignal("temperature_celsius", 72.4)
	alarms := rx.Map(
		rx.Filter(temperature.Subscribe(ctx), func(value float64) bool {
			return value >= 80
		}),
		func(value float64) string {
			return fmt.Sprintf("high temperature: %.1f °C", value)
		},
	)

	temperature.Set(83.1)
	fmt.Println(<-alarms)
}
