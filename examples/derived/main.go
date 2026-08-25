package main

import (
	"fmt"

	"github.com/webermarci/sup/rx"
)

func main() {
	celsius := rx.NewSignal("temperature_celsius", 25.0)
	fahrenheit := rx.NewDerived("temperature_fahrenheit", func() float64 {
		return celsius.Value()*9/5 + 32
	}, celsius)

	fmt.Printf("%.1f °C = %.1f °F\n", celsius.Value(), fahrenheit.Value())
}
