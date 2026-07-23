package rx_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/sup/rx"
)

func TestNestedPipelineClosesFromSubscriptionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	signal := rx.NewSignal("count", 0)
	output := rx.Map(
		rx.Filter(
			rx.Debounce(signal.Subscribe(ctx), time.Hour),
			func(value int) bool { return value >= 0 },
		),
		func(value int) string { return "value" },
	)

	cancel()

	select {
	case _, ok := <-output:
		if ok {
			t.Fatal("expected nested pipeline to close without flushing")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for nested pipeline to close")
	}
}
