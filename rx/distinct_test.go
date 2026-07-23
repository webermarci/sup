package rx_test

import (
	"testing"

	"github.com/webermarci/sup/rx"
)

func TestDistinctDropsConsecutiveEqualValues(t *testing.T) {
	input := make(chan int)
	output := rx.Distinct(input)

	input <- 1
	if got := receive(t, output); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	input <- 1
	input <- 2
	if got := receive(t, output); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
	input <- 2
	input <- 1
	if got := receive(t, output); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	close(input)
}

func TestDistinctFuncUsesLastEmittedValue(t *testing.T) {
	input := make(chan float64)
	output := rx.DistinctFunc(input, func(left, right float64) bool {
		difference := left - right
		if difference < 0 {
			difference = -difference
		}
		return difference < 0.1
	})

	input <- 0
	if got := receive(t, output); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	input <- 0.09
	input <- 0.18
	if got := receive(t, output); got != 0.18 {
		t.Fatalf("expected 0.18, got %v", got)
	}
	close(input)
}

func TestDistinctFuncRejectsNilEquality(t *testing.T) {
	defer expectPanic(t)
	rx.DistinctFunc[int](make(chan int), nil)
}
