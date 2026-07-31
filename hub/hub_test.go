package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/rx"
)

func TestHubDoesNotServeDebugFrontend(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/debug", nil)
	rec := httptest.NewRecorder()
	New("hub").Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHubExposesOnlyRegisteredActors(t *testing.T) {
	exposed := sup.ActorFunc("exposed", func(context.Context) error { return nil })
	internal := sup.ActorFunc("internal", func(context.Context) error { return nil })
	h := New("hub", WithActor(exposed))
	root := sup.NewSupervisor("root",
		sup.WithEventSink(h),
		sup.WithActors(exposed, internal),
		sup.WithPolicy(sup.Temporary),
	)

	if err := root.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/actors", nil)
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var actors []hubActor
	if err := json.NewDecoder(rec.Body).Decode(&actors); err != nil {
		t.Fatal(err)
	}
	if len(actors) != 1 || actors[0].ID != exposed.ID() {
		t.Fatalf("unexpected exposed actors: %#v", actors)
	}

	for _, event := range h.eventsSnapshot() {
		if event.SourceID == internal.ID() {
			t.Fatalf("internal actor event was exposed: %#v", event)
		}
	}
}

func TestHubBuildsPrunedSupervisionGraphFromEvents(t *testing.T) {
	exposed := sup.ActorFunc("worker.exposed", func(context.Context) error { return nil })
	internal := sup.ActorFunc("worker.internal", func(context.Context) error { return nil })
	exposedBranch := sup.NewSupervisor("branch.exposed",
		sup.WithActors(exposed),
		sup.WithPolicy(sup.Temporary),
	)
	internalBranch := sup.NewSupervisor("branch.internal",
		sup.WithActors(internal),
		sup.WithPolicy(sup.Temporary),
	)
	h := New("hub", WithActor(exposed))
	root := sup.NewSupervisor("root",
		sup.WithEventSink(h),
		sup.WithActors(exposedBranch, internalBranch),
		sup.WithPolicy(sup.Temporary),
	)

	if err := root.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	graph := h.graphSnapshot()
	if got, want := graphNodeIDs(graph), []string{"branch.exposed", "root", "worker.exposed"}; !equalStrings(got, want) {
		t.Fatalf("expected graph nodes %v, got %v", want, got)
	}

	wantEdges := []graphEdge{
		{SupervisorID: "branch.exposed", ActorID: "worker.exposed"},
		{SupervisorID: "root", ActorID: "branch.exposed"},
	}
	if len(graph.Edges) != len(wantEdges) {
		t.Fatalf("expected graph edges %#v, got %#v", wantEdges, graph.Edges)
	}
	for i := range wantEdges {
		if graph.Edges[i] != wantEdges[i] {
			t.Fatalf("expected graph edges %#v, got %#v", wantEdges, graph.Edges)
		}
	}
}

func TestHubCanProjectItsOwnRegistration(t *testing.T) {
	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	h := New("hub", WithActor(worker))
	root := sup.NewSupervisor("root",
		sup.WithEventSink(h),
		sup.WithActors(worker, h),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- root.Run(ctx) }()

	waitFor(t, func() bool {
		h.mu.RLock()
		defer h.mu.RUnlock()
		return h.parents[h.ID()] == root.ID()
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestHubHandleGraph(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	h := New("hub", WithActor(actor))
	root := sup.NewSupervisor("root",
		sup.WithEventSink(h),
		sup.WithActors(actor),
		sup.WithPolicy(sup.Temporary),
	)
	if err := root.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var graph supervisionGraph
	if err := json.NewDecoder(rec.Body).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Fatalf("unexpected graph: %#v", graph)
	}
	if graph.Edges[0] != (graphEdge{SupervisorID: "root", ActorID: "worker"}) {
		t.Fatalf("unexpected graph edge: %#v", graph.Edges[0])
	}
}

func TestHubReadsSignalValueDirectly(t *testing.T) {
	status := rx.NewSignal("status", "offline")
	h := New("hub", WithSignal(status))
	status.Set("online")

	req := httptest.NewRequest(http.MethodGet, "/signals/status", nil)
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var signal hubSignal
	if err := json.NewDecoder(rec.Body).Decode(&signal); err != nil {
		t.Fatal(err)
	}
	if signal.Value != "online" || signal.Writable {
		t.Fatalf("unexpected signal: %#v", signal)
	}
}

func TestHubSetsExplicitlyWritableSignal(t *testing.T) {
	target := rx.NewSignal("target", 21.0)
	h := New("hub", WithWritableSignal(target))

	req := httptest.NewRequest(http.MethodPatch, "/signals/target", strings.NewReader(`{"value":23.5}`))
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if target.Value() != 23.5 {
		t.Fatalf("expected target 23.5, got %v", target.Value())
	}

	var signal hubSignal
	if err := json.NewDecoder(rec.Body).Decode(&signal); err != nil {
		t.Fatal(err)
	}
	if !signal.Writable || signal.Value != 23.5 {
		t.Fatalf("unexpected signal response: %#v", signal)
	}
}

func TestHubRejectsWriteToReadOnlySignal(t *testing.T) {
	status := rx.NewSignal("status", "offline")
	h := New("hub", WithSignal(status))

	req := httptest.NewRequest(http.MethodPatch, "/signals/status", strings.NewReader(`{"value":"online"}`))
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d: %s", rec.Code, rec.Body.String())
	}
	if status.Value() != "offline" {
		t.Fatalf("read-only signal changed to %q", status.Value())
	}
}

func TestHubSignalEventsExcludeInitialValue(t *testing.T) {
	status := rx.NewSignal("status", "offline")
	observed := make(chan struct{})
	signal := &observedSignal[string]{
		Signal:   status,
		observed: observed,
	}
	h := New("hub", WithSignal[string](signal))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx) }()

	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("hub did not subscribe to signal")
	}
	if events := h.eventsSnapshot(); len(events) != 0 {
		t.Fatalf("initial value produced events: %#v", events)
	}

	status.Set("online")
	waitFor(t, func() bool {
		events := h.eventsSnapshot()
		return len(events) == 1 && events[0].Type == EventSignalUpdated
	})

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHubEventHistoryLimitIsGlobal(t *testing.T) {
	h := New("hub", WithEventHistoryLimit(2))
	h.publish(newHubEvent(sup.EventActorStarted, "one", nil, time.Time{}))
	h.publish(newHubEvent(sup.EventActorStopped, "two", nil, time.Time{}))
	h.publish(newHubEvent(sup.EventActorRestarting, "three", nil, time.Time{}))

	events := h.eventsSnapshot()
	if len(events) != 2 || events[0].SourceID != "two" || events[1].SourceID != "three" {
		t.Fatalf("unexpected retained events: %#v", events)
	}
}

func TestHubSnapshotsEventPayload(t *testing.T) {
	h := New("hub")
	value := map[string]any{
		"count":  1,
		"nested": []any{"original"},
	}

	h.publish(newHubEvent(
		EventSignalUpdated,
		"state",
		signalUpdatedPayload{Value: value},
		time.Time{},
	))

	value["count"] = 2
	value["nested"].([]any)[0] = "changed"

	events := h.eventsSnapshot()
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}

	payload, ok := events[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected snapshotted payload, got %T", events[0].Payload)
	}
	snapshot, ok := payload["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected snapshotted value, got %T", payload["value"])
	}
	if snapshot["count"] != float64(1) {
		t.Fatalf("expected original count, got %v", snapshot["count"])
	}
	nested, ok := snapshot["nested"].([]any)
	if !ok || len(nested) != 1 || nested[0] != "original" {
		t.Fatalf("expected original nested value, got %#v", snapshot["nested"])
	}
}

func TestHubZeroEventHistoryStillPublishesLive(t *testing.T) {
	h := New("hub", WithEventHistoryLimit(0))
	client := make(chan []byte, 1)
	h.clients[client] = struct{}{}

	h.publish(newHubEvent(sup.EventActorStarted, "worker", nil, time.Time{}))

	if len(h.eventsSnapshot()) != 0 {
		t.Fatal("expected event history to be disabled")
	}
	select {
	case <-client:
	default:
		t.Fatal("expected event to be published to live client")
	}
}

func TestHubHandleEventsReturnsChronologicalArray(t *testing.T) {
	h := New("hub")
	h.publish(newHubEvent(sup.EventActorStarted, "one", nil, time.Unix(1, 0)))
	h.publish(newHubEvent(sup.EventActorStopped, "two", nil, time.Unix(2, 0)))

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var events []hubEvent
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].SourceID != "one" || events[1].SourceID != "two" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestHubSerializesRegistrationSpec(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil },
		sup.WithSpec(sup.Spec{Kind: "worker"}),
	)
	event := hubEventFromRuntime(sup.Event{
		Type:       sup.EventActorRegistered,
		Time:       time.Unix(123, 0),
		Actor:      actor,
		Supervisor: sup.NewSupervisor("root"),
	})

	payload, ok := event.Payload.(actorRegisteredPayload)
	if !ok {
		t.Fatalf("expected registration payload, got %T", event.Payload)
	}
	if payload.SupervisorID != "root" || payload.Spec.Kind != "worker" {
		t.Fatalf("unexpected registration payload: %#v", payload)
	}
}

func TestHubSerializesRuntimeRestartEvent(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	supervisor := sup.NewSupervisor("root")
	eventTime := time.Unix(123, 456000000)
	boom := errors.New("boom")

	event := hubEventFromRuntime(sup.Event{
		Type:         sup.EventActorRestarting,
		Time:         eventTime,
		Actor:        actor,
		Supervisor:   supervisor,
		Err:          boom,
		RestartCount: 3,
	})

	if event.ID == "" || event.Timestamp != eventTime.UnixMilli() {
		t.Fatalf("expected hub identity and timestamp, got %#v", event)
	}
	payload, ok := event.Payload.(actorRestartingPayload)
	if !ok {
		t.Fatalf("expected restart payload, got %T", event.Payload)
	}
	if payload.SupervisorID != "root" || payload.RestartCount != 3 || payload.LastError != "boom" {
		t.Fatalf("unexpected restart payload: %#v", payload)
	}
}

func TestHubRejectsConcurrentRun(t *testing.T) {
	h := New("hub")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx) }()
	waitFor(t, func() bool { return h.running.Load() })

	if err := h.Run(context.Background()); !errors.Is(err, ErrHubRunning) {
		t.Fatalf("expected ErrHubRunning, got %v", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHubInspectListsSignalDependencies(t *testing.T) {
	h := New("hub",
		WithSignal(rx.NewSignal("z", 0)),
		WithSignal(rx.NewSignal("a", 0)),
	)

	spec := h.Inspect()
	if !equalStrings(spec.Dependencies, []string{"a", "z"}) {
		t.Fatalf("unexpected dependencies: %v", spec.Dependencies)
	}
}

func TestHubValidation(t *testing.T) {
	actor := sup.ActorFunc("duplicate", func(context.Context) error { return nil })

	requirePanic(t, func() { New("") })
	requirePanic(t, func() { New("hub", nil) })
	requirePanic(t, func() { New("hub", WithActor(nil)) })
	requirePanic(t, func() { New("hub", WithActor(actor), WithActor(actor)) })
	requirePanic(t, func() {
		New("hub", WithActor(actor), WithSignal(rx.NewSignal("duplicate", 0)))
	})
	requirePanic(t, func() {
		New("hub", WithEventHistoryLimit(-1))
	})
}

func graphNodeIDs(graph supervisionGraph) []string {
	ids := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if condition() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition was not met")
		case <-ticker.C:
		}
	}
}

type observedSignal[V any] struct {
	*rx.Signal[V]
	observed chan struct{}
}

func (s *observedSignal[V]) Subscribe(ctx context.Context) <-chan V {
	values := s.Signal.Subscribe(ctx)
	select {
	case s.observed <- struct{}{}:
	default:
	}
	return values
}
