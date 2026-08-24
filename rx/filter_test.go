package rx_test

import (
	"testing"

	"github.com/webermarci/sup/rx"
)

func TestFilterEmitsAcceptedValues(t *testing.T) {
	input := make(chan int)
	output := rx.Filter(input, func(value int) bool { return value%2 == 0 })

	go func() {
		input <- 1
		input <- 2
	}()

	if got := receive(t, output); got != 2 {
		t.Fatalf("expected accepted value 2, got %d", got)
	}

	close(input)
	if value, ok := <-output; ok {
		t.Fatalf("unexpected additional value %d", value)
	}
}

func expectPanic(t testing.TB) {
	t.Helper()
	if recover() == nil {
		t.Fatal("expected panic")
	}
}
