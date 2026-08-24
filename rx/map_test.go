package rx_test

import (
	"strconv"
	"testing"

	"github.com/webermarci/sup/rx"
)

func TestMapTransformsToAnotherType(t *testing.T) {
	input := make(chan int, 1)
	output := rx.Map(input, strconv.Itoa)

	input <- 42
	if got := receive(t, output); got != "42" {
		t.Fatalf("expected 42, got %q", got)
	}

	close(input)
	if _, ok := <-output; ok {
		t.Fatal("expected output to close with input")
	}
}

func TestMapAndFilterNest(t *testing.T) {
	input := make(chan int, 1)
	output := rx.Map(
		rx.Filter(input, func(value int) bool { return value%2 == 0 }),
		strconv.Itoa,
	)

	input <- 1
	input <- 2
	if got := receive(t, output); got != "2" {
		t.Fatalf("expected nested result 2, got %q", got)
	}
}
