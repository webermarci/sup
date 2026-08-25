package main

import (
	"context"
	"fmt"

	"github.com/webermarci/sup/rx"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetTemperature := rx.NewSignal("target_temperature_celsius", 21.0)
	values := targetTemperature.Subscribe(ctx)

	fmt.Printf("Initial target: %.1f °C\n", <-values)

	targetTemperature.Set(22.5)
	fmt.Printf("New target: %.1f °C\n", <-values)

	targetTemperature.Update(func(current float64) float64 {
		return current + 0.5
	})
	fmt.Printf("Adjusted target: %.1f °C\n", <-values)
}
