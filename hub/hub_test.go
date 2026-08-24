package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/rx"
)

func TestHubServesDebugPageAtMountedPath(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	status := rx.NewSignal("status", "online")
	h := New("debug").RegisterActors(actor).RegisterSignal(status)

	mux := http.NewServeMux()
	mux.Handle("/nested/", http.StripPrefix("/nested", h))

	req := httptest.NewRequest(http.MethodGet, "/nested", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/nested/" {
		t.Fatalf("unexpected mount redirect: status %d, location %q", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/nested/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	for _, content := range []string{
		"<h1>sup/debug</h1>",
		"data-graph",
		"data-signals",
		"data-events",
		"<script>",
	} {
		if !strings.Contains(rec.Body.String(), content) {
			t.Errorf("page does not contain %q", content)
		}
	}
	if strings.Contains(rec.Body.String(), "<nav") || strings.Contains(rec.Body.String(), "<a ") {
		t.Error("page contains navigation links")
	}
	if strings.Contains(rec.Body.String(), "Hub:") {
		t.Error("page contains the hub id label")
	}
	if strings.Contains(rec.Body.String(), "worker") || strings.Contains(rec.Body.String(), "online") {
		t.Error("page contains server-rendered hub data")
	}
	graphIndex := strings.Index(rec.Body.String(), "<h2>Supervision Graph")
	signalsIndex := strings.Index(rec.Body.String(), "<h2>Signals")
	eventsIndex := strings.Index(rec.Body.String(), "<h2>Recent Events")
	if graphIndex < 0 || signalsIndex < 0 || eventsIndex < 0 ||
		!(graphIndex < signalsIndex && signalsIndex < eventsIndex) {
		t.Error("page sections are not in graph, signals, events order")
	}

	req = httptest.NewRequest(http.MethodGet, "/nested/graph", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected mounted API status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"worker"`) {
		t.Fatalf("mounted graph API does not contain worker: %s", rec.Body.String())
	}
}

func TestWriteJSONDoesNotCommitPartialResponseOnMarshalError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, struct {
		Written     string `json:"written"`
		Unsupported func() `json:"unsupported"`
	}{
		Written:     "must not leak",
		Unsupported: func() {},
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "must not leak") {
		t.Fatalf("marshal error response contains partial JSON: %s", rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected error content type: %q", contentType)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("unexpected cache control: %q", cacheControl)
	}
}

func TestHubExposesOnlyRegisteredActors(t *testing.T) {
	exposed := sup.ActorFunc("exposed", func(context.Context) error { return nil })
	internal := sup.ActorFunc("internal", func(context.Context) error { return nil })
	h := New("hub").RegisterActors(exposed)
	root := sup.NewSupervisor("root", sup.Temporary).
		AddActors(exposed, internal, h)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- root.Run(ctx) }()
	waitFor(t, func() bool { return len(h.eventsSnapshot()) >= 3 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var graph supervisionGraph
	if err := json.NewDecoder(rec.Body).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	foundExposed := false
	for _, node := range graph.Nodes {
		if node.ID == exposed.ID() {
			foundExposed = true
		}
		if node.ID == internal.ID() {
			t.Fatalf("internal actor was exposed in graph: %#v", graph)
		}
	}
	if !foundExposed {
		t.Fatalf("registered actor was absent from graph: %#v", graph)
	}

	for _, event := range h.eventsSnapshot() {
		if event.SourceID == internal.ID() {
			t.Fatalf("internal actor event was exposed: %#v", event)
		}
	}
}

func TestHubRegistersActorsInBatch(t *testing.T) {
	first := sup.ActorFunc("first", func(context.Context) error { return nil })
	second := sup.ActorFunc("second", func(context.Context) error { return nil })

	h := New("hub").RegisterActors(first, second)
	if got := graphNodeIDs(h.graphSnapshot()); !slices.Equal(got, []string{"first", "second"}) {
		t.Fatalf("unexpected graph nodes: %v", got)
	}
}

func TestHubBuildsPrunedSupervisionGraphFromEvents(t *testing.T) {
	exposed := sup.ActorFunc("worker.exposed", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	internal := sup.ActorFunc("worker.internal", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	exposedBranch := sup.NewSupervisor("branch.exposed", sup.Temporary).AddActors(exposed)
	internalBranch := sup.NewSupervisor("branch.internal", sup.Temporary).AddActors(internal)
	h := New("hub").RegisterActors(exposed)
	root := sup.NewSupervisor("root", sup.Temporary).
		AddActors(exposedBranch, internalBranch, h)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- root.Run(ctx) }()
	waitFor(t, func() bool {
		graph := h.graphSnapshot()
		return len(graph.Nodes) >= 3 && len(graph.Edges) >= 2
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	graph := h.graphSnapshot()
	if got, want := graphNodeIDs(graph), []string{"branch.exposed", "root", "worker.exposed"}; !slices.Equal(got, want) {
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

func TestHubProjectsActorLifecycleState(t *testing.T) {
	worker := sup.ActorFunc("worker", func(context.Context) error { return nil })
	root := sup.NewSupervisor("root", sup.Transient)
	h := New("hub").RegisterActors(worker)

	tests := []struct {
		eventType sup.EventType
		wantState actorState
	}{
		{sup.EventActorRegistered, actorStateRegistered},
		{sup.EventActorStarted, actorStateRunning},
		{sup.EventActorRestarting, actorStateRestarting},
		{sup.EventActorStopped, actorStateStopped},
	}

	for _, test := range tests {
		h.HandleEvent(sup.Event{
			Type:       test.eventType,
			Actor:      worker,
			Supervisor: root,
		})

		graph := h.graphSnapshot()
		var got actorState
		for _, node := range graph.Nodes {
			if node.ID == worker.ID() {
				got = node.State
				break
			}
		}
		if got != test.wantState {
			t.Fatalf("after %s: expected state %q, got %q", test.eventType, test.wantState, got)
		}
	}
}

func TestHubCanProjectItsOwnRegistration(t *testing.T) {
	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	h := New("hub").RegisterActors(worker)
	root := sup.NewSupervisor("root", sup.Transient).
		AddActors(worker, h)
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
	actor := sup.ActorFunc("worker", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	h := New("hub").RegisterActors(actor)
	root := sup.NewSupervisor("root", sup.Temporary).
		AddActors(actor, h)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- root.Run(ctx) }()
	waitFor(t, func() bool {
		return len(h.graphSnapshot().Edges) == 1
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("unexpected cache control: %q", cacheControl)
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
	h := New("hub").RegisterSignal(status)
	status.Set("online")

	req := httptest.NewRequest(http.MethodGet, "/signals/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

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

func TestHubListsSignalsInStableOrder(t *testing.T) {
	h := New("hub").
		RegisterSignal(rx.NewSignal("zeta", true)).
		RegisterWritableSignal(rx.NewSignal("alpha", 21)).
		RegisterSignal(rx.NewSignal("middle", map[string]int{"count": 1}))

	req := httptest.NewRequest(http.MethodGet, "/signals", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var signals []hubSignal
	if err := json.NewDecoder(rec.Body).Decode(&signals); err != nil {
		t.Fatal(err)
	}
	if got, want := len(signals), 3; got != want {
		t.Fatalf("expected %d signals, got %d", want, got)
	}
	if signals[0].ID != "alpha" || signals[1].ID != "middle" || signals[2].ID != "zeta" {
		t.Fatalf("signals were not sorted by id: %#v", signals)
	}
	if signals[0].Type != hubSignalValueTypeNumber || !signals[0].Writable || signals[0].Value != float64(21) {
		t.Fatalf("unexpected writable numeric signal: %#v", signals[0])
	}
	if signals[1].Type != hubSignalValueTypeJSON || signals[1].Writable {
		t.Fatalf("unexpected JSON signal: %#v", signals[1])
	}
	if signals[2].Type != hubSignalValueTypeBoolean || signals[2].Value != true {
		t.Fatalf("unexpected boolean signal: %#v", signals[2])
	}
}

func TestHubSetsExplicitlyWritableSignal(t *testing.T) {
	target := rx.NewSignal("target", 21.0)
	h := New("hub").RegisterWritableSignal(target)

	req := httptest.NewRequest(http.MethodPatch, "/signals/target", strings.NewReader(`{"value":23.5}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

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

func TestHubPatchWritableSignalPublishesUpdateEvent(t *testing.T) {
	target := rx.NewSignal("target", 21)
	observed := make(chan struct{})
	signal := &observedSignal[int]{
		Signal:   target,
		observed: observed,
	}
	h := New("hub").RegisterWritableSignal(signal)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx) }()

	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("hub did not subscribe to writable signal")
	}

	req := httptest.NewRequest(http.MethodPatch, "/signals/target", strings.NewReader(`{"value":23}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	waitFor(t, func() bool { return len(h.eventsSnapshot()) == 1 })
	events := h.eventsSnapshot()
	if events[0].Type != eventSignalUpdated || events[0].SourceID != "target" {
		t.Fatalf("unexpected signal event: %#v", events[0])
	}
	payload, ok := events[0].Payload.(map[string]any)
	if !ok || payload["value"] != float64(23) {
		t.Fatalf("unexpected signal event payload: %#v", events[0].Payload)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHubRejectsWriteToReadOnlySignal(t *testing.T) {
	status := rx.NewSignal("status", "offline")
	h := New("hub").RegisterSignal(status)

	req := httptest.NewRequest(http.MethodPatch, "/signals/status", strings.NewReader(`{"value":"online"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d: %s", rec.Code, rec.Body.String())
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, allow)
	}
	if status.Value() != "offline" {
		t.Fatalf("read-only signal changed to %q", status.Value())
	}
}

func TestHubRejectsCrossOriginSignalUpdate(t *testing.T) {
	target := rx.NewSignal("target", 21)
	h := New("hub").RegisterWritableSignal(target)
	req := httptest.NewRequest(http.MethodPatch, "https://hub.example/signals/target", strings.NewReader(`{"value":23}`))
	req.Header.Set("Origin", "https://attacker.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if target.Value() != 21 {
		t.Fatalf("cross-origin request changed target to %d", target.Value())
	}
}

func TestHubAllowsSameOriginSignalUpdate(t *testing.T) {
	target := rx.NewSignal("target", 21)
	h := New("hub").RegisterWritableSignal(target)
	req := httptest.NewRequest(http.MethodPatch, "https://hub.example/signals/target", strings.NewReader(`{"value":23}`))
	req.Header.Set("Origin", "https://hub.example")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if target.Value() != 23 {
		t.Fatalf("same-origin request left target at %d", target.Value())
	}
}

func TestHubRejectsOversizedSignalUpdate(t *testing.T) {
	target := rx.NewSignal("target", 21)
	h := New("hub").RegisterWritableSignal(target)
	body := `{"value":"` + strings.Repeat("x", int(maxSignalUpdateBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/signals/target", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d: %s", rec.Code, rec.Body.String())
	}
	if target.Value() != 21 {
		t.Fatalf("oversized request changed target to %d", target.Value())
	}
}

func TestHubHTTPErrorResponses(t *testing.T) {
	h := New("hub").RegisterWritableSignal(rx.NewSignal("count", 1))

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "unknown signal", method: http.MethodGet, path: "/signals/missing", status: http.StatusNotFound},
		{name: "patch unknown signal", method: http.MethodPatch, path: "/signals/missing", body: `{"value":2}`, status: http.StatusNotFound},
		{name: "malformed patch", method: http.MethodPatch, path: "/signals/count", body: `{"value":`, status: http.StatusBadRequest},
		{name: "trailing JSON value", method: http.MethodPatch, path: "/signals/count", body: `{"value":2} {}`, status: http.StatusBadRequest},
		{name: "unknown member", method: http.MethodPatch, path: "/signals/count", body: `{"value":2,"extra":true}`, status: http.StatusBadRequest},
		{name: "missing value", method: http.MethodPatch, path: "/signals/count", body: `{}`, status: http.StatusBadRequest},
		{name: "incompatible value", method: http.MethodPatch, path: "/signals/count", body: `{"value":"two"}`, status: http.StatusBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHubSignalEventsExcludeInitialValue(t *testing.T) {
	status := rx.NewSignal("status", "offline")
	observed := make(chan struct{})
	signal := &observedSignal[string]{
		Signal:   status,
		observed: observed,
	}
	h := New("hub").RegisterSignal(signal)
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
		return len(events) == 1 && events[0].Type == eventSignalUpdated
	})

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHubEventHistoryLimitIsGlobal(t *testing.T) {
	h := New("hub").SetEventHistoryLimit(2)
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
		eventSignalUpdated,
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
	h := New("hub").SetEventHistoryLimit(0)
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
	h := New("hub").SetEventHistoryLimit(7)
	h.publish(newHubEvent(sup.EventActorStarted, "one", nil, time.Unix(1, 0)))
	h.publish(newHubEvent(sup.EventActorStopped, "two", nil, time.Unix(2, 0)))

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if limit := rec.Header().Get("Event-History-Limit"); limit != "7" {
		t.Fatalf("unexpected event history limit header: %q", limit)
	}
	var events []hubEvent
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].SourceID != "one" || events[1].SourceID != "two" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestHubEventStreamPublishesAndRemovesClientOnCancel(t *testing.T) {
	h := New("hub")
	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil).WithContext(ctx)
	w := newStreamRecorder()
	done := make(chan struct{})

	go func() {
		h.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-w.flushed:
	case <-time.After(time.Second):
		t.Fatal("event stream did not flush its headers")
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	if cacheControl := w.Header().Get("Cache-Control"); cacheControl != "no-cache" {
		t.Fatalf("unexpected cache control: %q", cacheControl)
	}

	event := newHubEvent(sup.EventActorStarted, "worker", nil, time.Unix(1, 0))
	h.publish(event)
	w.waitForBody(t, "event: actor:started")
	w.waitForBody(t, `"source_id":"worker"`)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event stream did not stop after cancellation")
	}

	h.mu.RLock()
	clients := len(h.clients)
	h.mu.RUnlock()
	if clients != 0 {
		t.Fatalf("expected event stream client to be removed, got %d clients", clients)
	}
}

func TestHubSerializesRegistration(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	event := hubEventFromRuntime(sup.Event{
		Type:       sup.EventActorRegistered,
		Time:       time.Unix(123, 0),
		Actor:      actor,
		Supervisor: sup.NewSupervisor("root", sup.Transient),
	})

	payload, ok := event.Payload.(actorRegisteredPayload)
	if !ok {
		t.Fatalf("expected registration payload, got %T", event.Payload)
	}
	if payload.SupervisorID != "root" {
		t.Fatalf("unexpected registration payload: %#v", payload)
	}
}

func TestHubSerializesRuntimeRestartEvent(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	supervisor := sup.NewSupervisor("root", sup.Transient)
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

func TestHubEventIDsAreUnique(t *testing.T) {
	first := newHubEvent(sup.EventActorStarted, "worker", nil, time.Time{})
	second := newHubEvent(sup.EventActorStarted, "worker", nil, time.Time{})

	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("expected unique event ids, got %q and %q", first.ID, second.ID)
	}
}

func TestHubValidation(t *testing.T) {
	actor := sup.ActorFunc("duplicate", func(context.Context) error { return nil })

	requirePanic(t, func() { New("") })
	requirePanic(t, func() { New("hub").RegisterActors(actor).RegisterActors(actor) })
	requirePanic(t, func() { New("hub").RegisterActors(actor, actor) })
	requirePanic(t, func() {
		New("hub").
			RegisterSignal(rx.NewSignal("duplicate", 0)).
			RegisterSignal(rx.NewSignal("duplicate", 1))
	})
	requirePanic(t, func() {
		New("hub").RegisterActors(actor).RegisterSignal(rx.NewSignal("duplicate", 0))
	})
	requirePanic(t, func() {
		New("hub").SetEventHistoryLimit(-1)
	})
}

func graphNodeIDs(graph supervisionGraph) []string {
	ids := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		ids = append(ids, node.ID)
	}
	return ids
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

type streamRecorder struct {
	header    http.Header
	body      strings.Builder
	flushed   chan struct{}
	flushOnce sync.Once
	mu        sync.Mutex
}

func newStreamRecorder() *streamRecorder {
	return &streamRecorder{
		header:  make(http.Header),
		flushed: make(chan struct{}),
	}
}

func (r *streamRecorder) Header() http.Header {
	return r.header
}

func (r *streamRecorder) WriteHeader(int) {}

func (r *streamRecorder) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(data)
}

func (r *streamRecorder) Flush() {
	r.flushOnce.Do(func() { close(r.flushed) })
}

func (r *streamRecorder) waitForBody(t *testing.T, content string) {
	t.Helper()
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return strings.Contains(r.body.String(), content)
	})
}

func (s *observedSignal[V]) Subscribe(ctx context.Context) <-chan V {
	values := s.Signal.Subscribe(ctx)
	select {
	case s.observed <- struct{}{}:
	default:
	}
	return values
}
