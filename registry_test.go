package sup

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	actor := ActorFunc("test-actor", func(ctx context.Context) error {
		return nil
	})

	r.Register(actor)
	got, ok := r.Get("test-actor")
	if !ok {
		t.Fatal("expected actor to be registered")
	}
	if got.Name() != "test-actor" {
		t.Errorf("expected name %q, got %q", "test-actor", got.Name())
	}

	actor2 := ActorFunc("test-actor", func(ctx context.Context) error {
		return nil
	})
	r.Register(actor2)
	got2, _ := r.Get("test-actor")
	if got2 != actor2 {
		t.Error("expected actor to be overwritten")
	}

	r.Unregister("test-actor")
	_, ok = r.Get("test-actor")
	if ok {
		t.Error("expected actor to be unregistered")
	}
}

func TestGlobalRegistry(t *testing.T) {
	r1 := GlobalRegistry()
	r2 := GlobalRegistry()

	if r1 != r2 {
		t.Error("GlobalRegistry should return the same instance")
	}

	actor := ActorFunc("global-actor", func(ctx context.Context) error {
		return nil
	})
	r1.Register(actor)
	defer r1.Unregister("global-actor")

	if _, ok := r2.Get("global-actor"); !ok {
		t.Error("actor registered in GlobalRegistry should be accessible from any GlobalRegistry() call")
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	r := NewRegistry()
	const count = 100
	var wg sync.WaitGroup

	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := range count {
			name := fmt.Sprintf("actor-%d", i)
			r.Register(ActorFunc(name, func(ctx context.Context) error {
				return nil
			}))
		}
	}()

	go func() {
		defer wg.Done()
		for i := range count {
			name := fmt.Sprintf("actor-%d", i)
			r.Get(name)
		}
	}()

	go func() {
		defer wg.Done()
		for i := range count {
			name := fmt.Sprintf("actor-%d", i)
			r.Unregister(name)
		}
	}()

	wg.Wait()
}
