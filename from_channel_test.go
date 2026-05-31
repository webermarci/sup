package sup_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestFromChannel_EmitsValues(t *testing.T) {
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2
	close(ch)

	var got []int
	err := sup.FromChannel(ch).Run(t.Context(), func(ctx context.Context, value int) error {
		got = append(got, value)
		return nil
	})
	if err != nil {
		t.Fatalf("expected source to exit cleanly, got %v", err)
	}

	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected emitted values %v, got %v", want, got)
	}
}

func TestFromChannel_ReturnsEmitError(t *testing.T) {
	expectedErr := errors.New("emit failed")

	ch := make(chan int, 1)
	ch <- 1

	err := sup.FromChannel(ch).Run(t.Context(), func(ctx context.Context, value int) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected emit error %v, got %v", expectedErr, err)
	}
}

func TestFromChannel_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ch := make(chan int)

	err := sup.FromChannel(ch).Run(ctx, func(ctx context.Context, value int) error {
		t.Fatalf("expected no emitted values, got %d", value)
		return nil
	})
	if err != nil {
		t.Fatalf("expected source to exit cleanly, got %v", err)
	}
}

func TestSignal_SourceFromChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := make(chan int)
	signal := sup.NewSignal("count", 0).
		Source(sup.FromChannel(ch))

	done := make(chan error, 1)
	go func() {
		done <- signal.Run(ctx)
	}()

	values := signal.Subscribe(ctx)

	select {
	case ch <- 42:
	case err := <-done:
		t.Fatalf("signal exited early: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out sending source value")
	}

	select {
	case got := <-values:
		if got != 42 {
			t.Fatalf("expected source value 42, got %d", got)
		}

	case err := <-done:
		t.Fatalf("signal exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for source update")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal shutdown")
	}
}
