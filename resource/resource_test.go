package resource_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/webermarci/sup/resource"
)

type testResource struct {
	value int
}

type releaseContextKey struct{}

type releaseObservation struct {
	err   error
	value any
}

func TestCallAndCast(t *testing.T) {
	released := make(chan testResource, 1)
	actor := resource.NewActor(
		"device",
		func(context.Context) (testResource, error) {
			return testResource{value: 21}, nil
		},
		func(ctx context.Context, value testResource) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			released <- value
			return nil
		},
	)

	if actor.ID() != "device" {
		t.Fatalf("ID() = %q, want device", actor.ID())
	}

	cancel, done := startActor(t, actor)
	value, err := resource.Call(t.Context(), actor, func(ctx context.Context, value testResource) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return value.value * 2, nil
	})
	if err != nil || value != 42 {
		t.Fatalf("Call() = (%d, %v), want (42, nil)", value, err)
	}

	castRan := make(chan struct{})
	if err := resource.Cast(t.Context(), actor, func(context.Context, testResource) error {
		close(castRan)
		return nil
	}); err != nil {
		t.Fatalf("Cast() = %v, want nil", err)
	}
	select {
	case <-castRan:
	case <-time.After(time.Second):
		t.Fatal("Cast operation did not run")
	}

	cancel()
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("Run() = %v, want nil after cancellation", err)
	}
	select {
	case value := <-released:
		if value.value != 21 {
			t.Fatalf("released value = %#v, want value 21", value)
		}
	case <-time.After(time.Second):
		t.Fatal("resource was not released")
	}
}

func TestCallFailureReleasesAndReacquires(t *testing.T) {
	acquired := make(chan int, 2)
	released := make(chan int, 2)
	var next atomic.Int32
	actor := resource.NewActor(
		"device",
		func(context.Context) (int, error) {
			value := int(next.Add(1))
			acquired <- value
			return value, nil
		},
		func(_ context.Context, value int) error {
			released <- value
			return nil
		},
	)

	_, firstDone := startActor(t, actor)
	if got := awaitInt(t, acquired); got != 1 {
		t.Fatalf("first acquired resource = %d, want 1", got)
	}
	wantErr := errors.New("read failed")
	value, err := resource.Call(t.Context(), actor, func(context.Context, int) (int, error) {
		return 0, wantErr
	})
	if value != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("Call() = (%d, %v), want (0, read failed)", value, err)
	}
	if err := awaitRun(t, firstDone); !errors.Is(err, wantErr) {
		t.Fatalf("first Run() = %v, want read failed", err)
	}
	if got := awaitInt(t, released); got != 1 {
		t.Fatalf("first released resource = %d, want 1", got)
	}

	cancel, secondDone := startActor(t, actor)
	if got := awaitInt(t, acquired); got != 2 {
		t.Fatalf("second acquired resource = %d, want 2", got)
	}
	value, err = resource.Call(t.Context(), actor, func(context.Context, int) (int, error) {
		return 7, nil
	})
	if err != nil || value != 7 {
		t.Fatalf("second Call() = (%d, %v), want (7, nil)", value, err)
	}
	cancel()
	if err := awaitRun(t, secondDone); err != nil {
		t.Fatalf("second Run() = %v, want nil after cancellation", err)
	}
	if got := awaitInt(t, released); got != 2 {
		t.Fatalf("second released resource = %d, want 2", got)
	}
}

func TestCallReturnsExecutionStoppedWhenOperationPanics(t *testing.T) {
	acquired := make(chan struct{})
	released := make(chan struct{})
	actor := resource.NewActor(
		"device",
		func(context.Context) (int, error) {
			close(acquired)
			return 1, nil
		},
		func(context.Context, int) error {
			close(released)
			return nil
		},
	)

	panicValue := make(chan any, 1)
	go func() {
		defer func() { panicValue <- recover() }()
		_ = actor.Run(t.Context())
	}()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("resource was not acquired")
	}

	_, err := resource.Call(t.Context(), actor, func(context.Context, int) (int, error) {
		panic("operation panicked")
	})
	if !errors.Is(err, resource.ErrExecutionStopped) {
		t.Fatalf("Call() = %v, want ErrExecutionStopped", err)
	}
	select {
	case value := <-panicValue:
		if value != "operation panicked" {
			t.Fatalf("panic value = %v, want operation panicked", value)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not propagate the panic")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("resource was not released")
	}
}

func TestOperationsWaitBetweenExecutions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		acquired := make(chan int, 2)
		var next atomic.Int32
		actor := resource.NewActor(
			"device",
			func(context.Context) (int, error) {
				value := int(next.Add(1))
				acquired <- value
				return value, nil
			},
			func(context.Context, int) error { return nil },
		)

		_, firstDone := startActor(t, actor)
		if got := awaitInt(t, acquired); got != 1 {
			t.Fatalf("first acquired resource = %d, want 1", got)
		}
		wantErr := errors.New("connection lost")
		if _, err := resource.Call(t.Context(), actor, func(context.Context, int) (int, error) {
			return 0, wantErr
		}); !errors.Is(err, wantErr) {
			t.Fatalf("failing Call() = %v, want connection lost", err)
		}
		if err := awaitRun(t, firstDone); !errors.Is(err, wantErr) {
			t.Fatalf("first Run() = %v, want connection lost", err)
		}

		type callResult struct {
			value int
			err   error
		}
		callDone := make(chan callResult, 1)
		castDone := make(chan error, 1)
		castRan := make(chan struct{})
		go func() {
			value, err := resource.Call(t.Context(), actor, func(_ context.Context, value int) (int, error) {
				return value, nil
			})
			callDone <- callResult{value: value, err: err}
		}()
		go func() {
			castDone <- resource.Cast(t.Context(), actor, func(context.Context, int) error {
				close(castRan)
				return nil
			})
		}()
		synctest.Wait()
		select {
		case result := <-callDone:
			t.Fatalf("Call() completed between executions: %#v", result)
		default:
		}
		select {
		case err := <-castDone:
			t.Fatalf("Cast() completed between executions: %v", err)
		default:
		}

		cancel, secondDone := startActor(t, actor)
		if got := awaitInt(t, acquired); got != 2 {
			t.Fatalf("second acquired resource = %d, want 2", got)
		}
		select {
		case result := <-callDone:
			if result.value != 2 || result.err != nil {
				t.Fatalf("Call() = (%d, %v), want (2, nil)", result.value, result.err)
			}
		case <-time.After(time.Second):
			t.Fatal("Call() did not continue on the next execution")
		}
		if err := awaitResult(t, castDone); err != nil {
			t.Fatalf("Cast() = %v, want nil", err)
		}
		select {
		case <-castRan:
		case <-time.After(time.Second):
			t.Fatal("Cast operation did not run on the next execution")
		}

		cancel()
		if err := awaitRun(t, secondDone); err != nil {
			t.Fatalf("second Run() = %v, want nil after cancellation", err)
		}
	})
}

func TestCallWaitsAcrossAcquisitionFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts atomic.Int32
		wantErr := errors.New("connection refused")
		actor := resource.NewActor(
			"device",
			func(context.Context) (int, error) {
				if attempts.Add(1) == 1 {
					return 0, wantErr
				}
				return 42, nil
			},
			func(context.Context, int) error { return nil },
		)

		result := make(chan error, 1)
		go func() {
			value, err := resource.Call(t.Context(), actor, func(_ context.Context, value int) (int, error) {
				return value, nil
			})
			if err == nil && value != 42 {
				err = fmt.Errorf("value = %d, want 42", value)
			}
			result <- err
		}()

		_, firstDone := startActor(t, actor)
		if err := awaitRun(t, firstDone); !errors.Is(err, wantErr) {
			t.Fatalf("first Run() = %v, want connection refused", err)
		}
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("Call() completed after failed acquisition: %v", err)
		default:
		}

		cancel, secondDone := startActor(t, actor)
		if err := awaitResult(t, result); err != nil {
			t.Fatalf("Call() = %v, want nil", err)
		}
		cancel()
		if err := awaitRun(t, secondDone); err != nil {
			t.Fatalf("second Run() = %v, want nil after cancellation", err)
		}
	})
}

func TestOperationsAreSerialized(t *testing.T) {
	actor := resource.NewActor(
		"device",
		func(context.Context) (testResource, error) { return testResource{}, nil },
		func(context.Context, testResource) error { return nil },
	)
	cancel, done := startActor(t, actor)

	var active atomic.Int32
	var maximum atomic.Int32
	updateMaximum := func(value int32) {
		for {
			old := maximum.Load()
			if value <= old || maximum.CompareAndSwap(old, value) {
				return
			}
		}
	}

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	results := make(chan error, 3)
	for i := range 3 {
		go func(index int) {
			_, err := resource.Call(t.Context(), actor, func(context.Context, testResource) (int, error) {
				current := active.Add(1)
				updateMaximum(current)
				if index == 0 {
					close(firstStarted)
					<-releaseFirst
				}
				active.Add(-1)
				return index, nil
			})
			results <- err
		}(i)
	}

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first operation did not run")
	}
	close(releaseFirst)
	for range 3 {
		if err := awaitResult(t, results); err != nil {
			t.Fatalf("Call() = %v, want nil", err)
		}
	}
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent operations = %d, want 1", got)
	}

	cancel()
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("Run() = %v, want nil after cancellation", err)
	}
}

func TestOperationHandoffHonorsCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		actor := resource.NewActor(
			"device",
			func(context.Context) (testResource, error) { return testResource{}, nil },
			func(context.Context, testResource) error { return nil },
		)
		cancelActor, done := startActor(t, actor)

		activeStarted := make(chan struct{})
		releaseActive := make(chan struct{})
		if err := resource.Cast(t.Context(), actor, func(context.Context, testResource) error {
			close(activeStarted)
			<-releaseActive
			return nil
		}); err != nil {
			t.Fatalf("active Cast() = %v, want nil", err)
		}
		select {
		case <-activeStarted:
		case <-time.After(time.Second):
			t.Fatal("active operation did not run")
		}

		callCtx, cancelCall := context.WithCancel(t.Context())
		blocked := make(chan error, 1)
		go func() {
			blocked <- resource.Cast(callCtx, actor, func(context.Context, testResource) error { return nil })
		}()
		synctest.Wait()
		cancelCall()
		if err := awaitResult(t, blocked); !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked Cast() = %v, want context.Canceled", err)
		}

		close(releaseActive)
		cancelActor()
		if err := awaitRun(t, done); err != nil {
			t.Fatalf("Run() = %v, want nil after cancellation", err)
		}
	})
}

func TestCastFailureReleasesAndReacquires(t *testing.T) {
	acquired := make(chan int, 2)
	released := make(chan int, 2)
	var next atomic.Int32
	actor := resource.NewActor(
		"device",
		func(context.Context) (int, error) {
			value := int(next.Add(1))
			acquired <- value
			return value, nil
		},
		func(ctx context.Context, value int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			released <- value
			return nil
		},
	)

	_, firstDone := startActor(t, actor)
	if got := awaitInt(t, acquired); got != 1 {
		t.Fatalf("first acquired resource = %d, want 1", got)
	}
	wantErr := errors.New("connection lost")
	if err := resource.Cast(t.Context(), actor, func(context.Context, int) error { return wantErr }); err != nil {
		t.Fatalf("failing Cast() = %v, want nil handoff result", err)
	}
	if err := awaitRun(t, firstDone); !errors.Is(err, wantErr) {
		t.Fatalf("first Run() = %v, want connection lost", err)
	}
	if got := awaitInt(t, released); got != 1 {
		t.Fatalf("first released resource = %d, want 1", got)
	}

	cancel, secondDone := startActor(t, actor)
	if got := awaitInt(t, acquired); got != 2 {
		t.Fatalf("second acquired resource = %d, want 2", got)
	}
	cancel()
	if err := awaitRun(t, secondDone); err != nil {
		t.Fatalf("second Run() = %v, want nil after cancellation", err)
	}
	if got := awaitInt(t, released); got != 2 {
		t.Fatalf("second released resource = %d, want 2", got)
	}
}

func TestReleaseGetsFreshContext(t *testing.T) {
	acquired := make(chan struct{})
	observations := make(chan releaseObservation, 1)
	actor := resource.NewActor(
		"device",
		func(context.Context) (testResource, error) {
			close(acquired)
			return testResource{}, nil
		},
		func(ctx context.Context, _ testResource) error {
			observations <- releaseObservation{
				err:   ctx.Err(),
				value: ctx.Value(releaseContextKey{}),
			}
			return nil
		},
	)
	baseCtx := context.WithValue(t.Context(), releaseContextKey{}, "preserved")
	ctx, cancel := context.WithCancel(baseCtx)
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("resource was not acquired")
	}
	cancel()
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("Run() = %v, want nil after cancellation", err)
	}
	select {
	case observation := <-observations:
		if observation.err != nil {
			t.Fatalf("release context error = %v, want nil", observation.err)
		}
		if observation.value != "preserved" {
			t.Fatalf("release context value = %v, want preserved", observation.value)
		}
	case <-time.After(time.Second):
		t.Fatal("release was not called")
	}
}

func TestValidation(t *testing.T) {
	requirePanic(t, func() {
		resource.NewActor("", func(context.Context) (int, error) { return 0, nil }, func(context.Context, int) error { return nil })
	})
	requirePanic(t, func() {
		resource.NewActor("actor", func(context.Context) (int, error) { return 0, nil }, func(context.Context, int) error { return nil }).SetReleaseTimeout(0)
	})
}

func startActor[T any](t *testing.T, actor *resource.Actor[T]) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()
	return cancel, done
}

func awaitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("actor did not stop")
		return nil
	}
}

func awaitResult(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(time.Second):
		t.Fatal("operation did not finish")
		return nil
	}
}

func awaitInt(t *testing.T, values <-chan int) int {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("value was not produced")
		return 0
	}
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
