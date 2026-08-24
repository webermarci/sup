package rx_test

import (
	"testing"

	"github.com/webermarci/sup/rx"
)

func TestTransitionsEmitsDistinctChangesAfterInitialValue(t *testing.T) {
	input := make(chan int)
	output := rx.Transitions(input)

	input <- 1
	input <- 1
	input <- 2
	if got := receive(t, output); got != (rx.Transition[int]{From: 1, To: 2}) {
		t.Fatalf("unexpected transition: %#v", got)
	}

	input <- 3
	if got := receive(t, output); got != (rx.Transition[int]{From: 2, To: 3}) {
		t.Fatalf("unexpected transition: %#v", got)
	}

	close(input)
	if transition, ok := <-output; ok {
		t.Fatalf("unexpected additional transition: %#v", transition)
	}
}

func TestTransitionsFuncUsesLastDistinctValue(t *testing.T) {
	input := make(chan float64)
	output := rx.TransitionsFunc(input, func(left, right float64) bool {
		difference := left - right
		if difference < 0 {
			difference = -difference
		}
		return difference < 0.1
	})

	input <- 0
	input <- 0.09
	input <- 0.18
	if got := receive(t, output); got != (rx.Transition[float64]{From: 0, To: 0.18}) {
		t.Fatalf("unexpected transition: %#v", got)
	}

	close(input)
}

func TestEdgesEmitsRisingAndFallingEdges(t *testing.T) {
	input := make(chan bool)
	output := rx.Edges(input)

	input <- false
	input <- true
	if got := receive(t, output); got != rx.EdgeRising {
		t.Fatalf("expected rising edge, got %v", got)
	}

	input <- true
	input <- false
	if got := receive(t, output); got != rx.EdgeFalling {
		t.Fatalf("expected falling edge, got %v", got)
	}

	close(input)
	if edge, ok := <-output; ok {
		t.Fatalf("unexpected additional edge: %v", edge)
	}
}

func TestTransitionsClosesWithoutValues(t *testing.T) {
	input := make(chan string)
	output := rx.Transitions(input)
	close(input)

	if transition, ok := <-output; ok {
		t.Fatalf("unexpected transition: %#v", transition)
	}
}
