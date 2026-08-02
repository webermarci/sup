package rx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/webermarci/sup/rx"
)

func TestDerivedSupportsMultipleTypedDependencies(t *testing.T) {
	count := rx.NewSignal("count", 1)
	name := rx.NewSignal("name", "items")
	derived := rx.NewDerived("summary", func() string {
		return fmt.Sprintf("%d %s", count.Value(), name.Value())
	}, count, name)

	if got := derived.Value(); got != "1 items" {
		t.Fatalf("expected initial value, got %q", got)
	}

	ctx, cancel := context.WithCancel(t.Context())
	values := derived.Subscribe(ctx)
	<-values
	done := make(chan error, 1)
	go func() { done <- derived.Run(ctx) }()

	<-values // startup recomputation

	count.Set(2)
	if got := receive(t, values); got != "2 items" {
		t.Fatalf("expected count recomputation, got %q", got)
	}

	name.Set("things")
	if got := receive(t, values); got != "2 things" {
		t.Fatalf("expected name recomputation, got %q", got)
	}

	cancel()
	if err := receive(t, done); err != nil {
		t.Fatalf("derived shutdown failed: %v", err)
	}
}

func TestDerivedHasIndependentSubscribers(t *testing.T) {
	dependency := rx.NewSignal("dependency", 1)
	derived := rx.NewDerived("derived", func() int {
		return dependency.Value() * 2
	}, dependency)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go derived.Run(ctx)

	first := derived.Subscribe(ctx)
	second := derived.Subscribe(ctx)
	<-first
	<-second
	dependency.Set(2)

	if got := receive(t, first); got != 4 {
		t.Fatalf("first subscriber: expected 4, got %d", got)
	}
	if got := receive(t, second); got != 4 {
		t.Fatalf("second subscriber: expected 4, got %d", got)
	}
}

func TestDerivedCanDependOnDerived(t *testing.T) {
	root := rx.NewSignal("root", 1)
	doubled := rx.NewDerived("doubled", func() int {
		return root.Value() * 2
	}, root)
	tripled := rx.NewDerived("tripled", func() int {
		return doubled.Value() * 3
	}, doubled)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	doubledDone := make(chan error, 1)
	tripledDone := make(chan error, 1)
	go func() { doubledDone <- doubled.Run(ctx) }()
	go func() { tripledDone <- tripled.Run(ctx) }()

	values := tripled.Subscribe(ctx)
	root.Set(2)
	for {
		if got := receive(t, values); got == 12 {
			break
		}
	}

	cancel()
	if err := receive(t, doubledDone); err != nil {
		t.Fatalf("doubled shutdown failed: %v", err)
	}
	if err := receive(t, tripledDone); err != nil {
		t.Fatalf("tripled shutdown failed: %v", err)
	}
}

func TestDerivedRestartRecomputesChangesWhileStopped(t *testing.T) {
	dependency := rx.NewSignal("dependency", 1)
	derived := rx.NewDerived("derived", func() int {
		return dependency.Value() * 2
	}, dependency)
	values := derived.Subscribe(t.Context())
	<-values

	firstCtx, cancelFirst := context.WithCancel(t.Context())
	firstDone := make(chan error, 1)
	go func() { firstDone <- derived.Run(firstCtx) }()
	receive(t, values) // startup recomputation
	cancelFirst()
	if err := receive(t, firstDone); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	dependency.Set(2)
	if got := derived.Value(); got != 2 {
		t.Fatalf("stopped derived unexpectedly recomputed to %d", got)
	}

	secondCtx, cancelSecond := context.WithCancel(t.Context())
	secondDone := make(chan error, 1)
	go func() { secondDone <- derived.Run(secondCtx) }()
	if got := receive(t, values); got != 4 {
		t.Fatalf("expected restarted value 4, got %d", got)
	}
	cancelSecond()
	if err := receive(t, secondDone); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

func TestDerivedRejectsConcurrentRun(t *testing.T) {
	dependency := newObservedDependency("dependency", 0)
	derived := rx.NewDerived("derived", func() int {
		return dependency.Value()
	}, dependency)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- derived.Run(ctx) }()
	receive(t, dependency.watched)

	if err := derived.Run(t.Context()); !errors.Is(err, rx.ErrDerivedRunning) {
		t.Fatalf("expected ErrDerivedRunning, got %v", err)
	}

	cancel()
	if err := receive(t, done); err != nil {
		t.Fatalf("derived shutdown failed: %v", err)
	}
}

func TestDerivedInspect(t *testing.T) {
	first := rx.NewSignal("first", 1)
	second := rx.NewSignal("second", 2)
	derived := rx.NewDerived("sum", func() int {
		return first.Value() + second.Value()
	}, first, second)

	spec := derived.Inspect()
	if spec.Kind != "derived" {
		t.Fatalf("expected derived kind, got %q", spec.Kind)
	}
	if len(spec.Dependencies) != 2 ||
		spec.Dependencies[0] != "first" ||
		spec.Dependencies[1] != "second" {
		t.Fatalf("unexpected dependencies %v", spec.Dependencies)
	}
}

func TestDerivedValidatesArguments(t *testing.T) {
	tests := map[string]func(){
		"empty id":       func() { rx.NewDerived("", func() int { return 0 }) },
		"nil compute":    func() { rx.NewDerived[int]("derived", nil) },
		"nil dependency": func() { rx.NewDerived("derived", func() int { return 0 }, nil) },
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defer expectPanic(t)
			test()
		})
	}
}

func TestDerivedRunRejectsNilContext(t *testing.T) {
	derived := rx.NewDerived("derived", func() int { return 0 })
	defer expectPanic(t)
	derived.Run(nil)
}

type observedDependency struct {
	*rx.Signal[int]
	watched chan struct{}
}

func newObservedDependency(id string, initial int) *observedDependency {
	return &observedDependency{
		Signal:  rx.NewSignal(id, initial),
		watched: make(chan struct{}, 1),
	}
}

func (d *observedDependency) Watch(ctx context.Context) <-chan struct{} {
	select {
	case d.watched <- struct{}{}:
	default:
	}
	return d.Signal.Watch(ctx)
}
