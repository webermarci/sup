package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

type testActor struct {
	*sup.BaseActor
}

func newTestActor(id string) *testActor {
	return &testActor{BaseActor: sup.NewBaseActor(id)}
}

func (a *testActor) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (a *testActor) Inspect() sup.Spec {
	return sup.Spec{
		Kind: "test_actor",
	}
}

type incrementInput struct {
	Amount int `json:"amount"`
}

type countOutput struct {
	Count int `json:"count"`
}

func TestRegistryIndex(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Cast("increment", func(ctx context.Context, in incrementInput) error {
				return nil
			}),
			Call("get", func(ctx context.Context, in struct{}) (countOutput, error) {
				return countOutput{Count: 42}, nil
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Actors  []ExposedActor  `json:"actors"`
		Signals []ExposedSignal `json:"signals"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if len(body.Actors) != 1 {
		t.Fatalf("expected 1 actor, got %d", len(body.Actors))
	}

	if body.Actors[0].ID != "counter" {
		t.Fatalf("expected actor id counter, got %q", body.Actors[0].ID)
	}

	if body.Actors[0].Control == nil {
		t.Fatal("expected actor control")
	}

	if len(body.Actors[0].Control.Casts) != 1 {
		t.Fatalf("expected 1 cast, got %d", len(body.Actors[0].Control.Casts))
	}

	if len(body.Actors[0].Control.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(body.Actors[0].Control.Calls))
	}
}

func TestRegistryCast(t *testing.T) {
	actor := newTestActor("counter")
	var count int

	registry := NewRegistry("registry",
		WithActor(actor,
			Cast("increment", func(ctx context.Context, in incrementInput) error {
				count += in.Amount
				return nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/casts/increment",
		strings.NewReader(`{"amount":5}`),
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if count != 5 {
		t.Fatalf("expected count 5, got %d", count)
	}
}

func TestRegistryCall(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Call("get", func(ctx context.Context, in struct{}) (countOutput, error) {
				return countOutput{Count: 42}, nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/calls/get",
		nil,
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out countOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if out.Count != 42 {
		t.Fatalf("expected count 42, got %d", out.Count)
	}
}

func TestRegistryUnknownActor(t *testing.T) {
	registry := NewRegistry("registry")

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/missing/casts/reset",
		nil,
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestRegistryActorWithoutControl(t *testing.T) {
	actor := newTestActor("worker")

	registry := NewRegistry("registry",
		WithActor(actor),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/worker/casts/reset",
		nil,
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestRegistryDuplicateActorPanics(t *testing.T) {
	actor := newTestActor("worker")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = NewRegistry("registry",
		WithActor(actor),
		WithActor(actor),
	)
}

func TestRegistryDuplicateSignalPanics(t *testing.T) {
	signal := sup.NewPushedSignal("status", func(ctx context.Context, v string) error {
		return nil
	})

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = NewRegistry("registry",
		WithSignal(signal),
		WithSignal(signal),
	)
}

func TestRegistrySignalIndex(t *testing.T) {
	signal := sup.NewPushedSignal("status", func(ctx context.Context, v string) error {
		return nil
	})

	registry := NewRegistry("registry",
		WithSignal(signal),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Actors  []ExposedActor  `json:"actors"`
		Signals []ExposedSignal `json:"signals"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if len(body.Signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(body.Signals))
	}

	if body.Signals[0].ID != "status" {
		t.Fatalf("expected signal id status, got %q", body.Signals[0].ID)
	}
}

func TestRegistrySignalSSE(t *testing.T) {
	signal := sup.NewPushedSignal("status", func(ctx context.Context, v string) error {
		return nil
	})

	registry := NewRegistry("registry",
		WithSignal(signal),
	)

	ctx := t.Context()

	go func() {
		_ = signal.Run(ctx)
	}()

	go func() {
		_ = registry.Run(ctx)
	}()

	ch := make(chan []byte, 1)

	registry.mu.Lock()
	registry.clients[ch] = struct{}{}
	registry.mu.Unlock()

	defer func() {
		registry.mu.Lock()
		delete(registry.clients, ch)
		registry.mu.Unlock()
		close(ch)
	}()

	time.Sleep(50 * time.Millisecond)

	if err := signal.Write(context.Background(), "online"); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ch:
		s := string(msg)

		if !strings.Contains(s, "event: signal:update") {
			t.Fatalf("expected signal:update event, got:\n%s", s)
		}
		if !strings.Contains(s, `"id":"status"`) {
			t.Fatalf("expected status id in:\n%s", s)
		}
		if !strings.Contains(s, `"value":"online"`) {
			t.Fatalf("expected online value in:\n%s", s)
		}

	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for signal update")
	}
}

func TestDuplicateCastPanics(t *testing.T) {
	actor := newTestActor("counter")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = NewRegistry("registry",
		WithActor(actor,
			Cast("reset", func(ctx context.Context, in struct{}) error {
				return nil
			}),
			Cast("reset", func(ctx context.Context, in struct{}) error {
				return nil
			}),
		),
	)
}

func TestDuplicateCallPanics(t *testing.T) {
	actor := newTestActor("counter")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = NewRegistry("registry",
		WithActor(actor,
			Call("get", func(ctx context.Context, in struct{}) (int, error) {
				return 1, nil
			}),
			Call("get", func(ctx context.Context, in struct{}) (int, error) {
				return 2, nil
			}),
		),
	)
}

func TestCastCallNameCollisionPanics(t *testing.T) {
	actor := newTestActor("counter")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = NewRegistry("registry",
		WithActor(actor,
			Cast("state", func(ctx context.Context, in struct{}) error {
				return nil
			}),
			Call("state", func(ctx context.Context, in struct{}) (int, error) {
				return 1, nil
			}),
		),
	)
}

func TestRegistryCastInvalidJSON(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Cast("increment", func(ctx context.Context, in incrementInput) error {
				return nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/casts/increment",
		strings.NewReader(`{"amount":`),
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestRegistryCallInvalidJSON(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Call("get", func(ctx context.Context, in incrementInput) (int, error) {
				return in.Amount, nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/calls/get",
		strings.NewReader(`{"amount":`),
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestRegistryMissingCast(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Cast("increment", func(ctx context.Context, in incrementInput) error {
				return nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/casts/missing",
		strings.NewReader(`{"amount":1}`),
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestRegistryMissingCall(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Call("get", func(ctx context.Context, in struct{}) (int, error) {
				return 1, nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/calls/missing",
		nil,
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestRegistryCastEmptyBody(t *testing.T) {
	actor := newTestActor("counter")

	called := false

	registry := NewRegistry("registry",
		WithActor(actor,
			Cast("reset", func(ctx context.Context, in struct{}) error {
				called = true
				return nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/casts/reset",
		nil,
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if !called {
		t.Fatal("expected cast to be called")
	}
}

func TestRegistryCallEmptyBody(t *testing.T) {
	actor := newTestActor("counter")

	registry := NewRegistry("registry",
		WithActor(actor,
			Call("get", func(ctx context.Context, in struct{}) (countOutput, error) {
				return countOutput{Count: 7}, nil
			}),
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/actors/counter/calls/get",
		nil,
	)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out countOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if out.Count != 7 {
		t.Fatalf("expected count 7, got %d", out.Count)
	}
}

func TestRegistryRunStopsOnContextCancel(t *testing.T) {
	registry := NewRegistry("registry")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := registry.Run(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRegistrySignalIndexUpdatesValue(t *testing.T) {
	signal := sup.NewPushedSignal("status", func(ctx context.Context, v string) error {
		return nil
	})

	registry := NewRegistry("registry",
		WithSignal(signal),
	)

	ctx := t.Context()

	go func() {
		_ = signal.Run(ctx)
	}()

	go func() {
		_ = registry.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	if err := signal.Write(context.Background(), "online"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	registry.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Signals []ExposedSignal `json:"signals"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if len(body.Signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(body.Signals))
	}

	if body.Signals[0].Value != "online" {
		t.Fatalf("expected signal value online, got %#v", body.Signals[0].Value)
	}
}
