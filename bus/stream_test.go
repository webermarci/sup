package bus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStream_Basic(t *testing.T) {
	ctx := t.Context()

	source := make(chan int, 1)
	stream := NewStream("test", func(ctx context.Context) (<-chan int, error) {
		return source, nil
	}).WithInitialValue(0)

	go stream.Run(ctx)

	sub := stream.Subscribe(ctx)

	source <- 1
	select {
	case v := <-sub:
		if v != 1 {
			t.Errorf("expected 1, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for value")
	}

	if val := stream.Read(); val != 1 {
		t.Errorf("expected Read() to return 1, got %d", val)
	}
}

func TestStream_InitialNotify(t *testing.T) {
	ctx := t.Context()

	// Use a factory that doesn't fail but return a channel that will eventually get data
	source := make(chan int, 1)
	stream := NewStream("test", func(ctx context.Context) (<-chan int, error) {
		return source, nil
	}).WithInitialValue(42).WithInitialNotify(true)

	go stream.Run(ctx)

	sub := stream.Subscribe(ctx)
	select {
	case v := <-sub:
		if v != 42 {
			t.Errorf("expected initial value 42, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for initial notify")
	}
}

func TestStream_ExitOnClose(t *testing.T) {
	source := make(chan int, 1)
	stream := NewStream("test", func(ctx context.Context) (<-chan int, error) {
		return source, nil
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- stream.Run(t.Context())
	}()

	close(source)

	select {
	case err := <-errCh:
		if err == nil || err.Error() != "stream source closed" {
			t.Errorf("expected error 'stream source closed', got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for actor to exit")
	}
}

func TestStream_ExitOnFactoryError(t *testing.T) {
	stream := NewStream("test", func(ctx context.Context) (<-chan int, error) {
		return nil, errors.New("factory failed")
	})

	err := stream.Run(t.Context())
	if err == nil || err.Error() != "factory failed" {
		t.Errorf("expected error 'factory failed', got %v", err)
	}
}
