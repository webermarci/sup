package hub

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

type hubTestActor struct {
	*sup.BaseActor
	casts *sup.CastInbox[hubIncrement]
	calls *sup.CallInbox[hubGet, hubCount]
}

type hubIncrement struct {
	Amount int `json:"amount"`
}

type hubGet struct{}

type hubCount struct {
	Count int `json:"count"`
}

func newHubTestActor(id string) *hubTestActor {
	return &hubTestActor{
		BaseActor: sup.NewBaseActor(id),
		casts:     sup.NewCastInbox[hubIncrement](1),
		calls:     sup.NewCallInbox[hubGet, hubCount](1),
	}
}

func (a *hubTestActor) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (a *hubTestActor) Controls() []sup.Control {
	return []sup.Control{
		sup.NewCastControl[hubIncrement]("increment", a.casts),
		sup.NewCallControl[hubGet, hubCount]("get", a.calls),
	}
}

func TestHubHandleActors(t *testing.T) {
	hub := New("hub", WithActor(newHubTestActor("counter")))

	req := httptest.NewRequest(http.MethodGet, "/actors", nil)
	rec := httptest.NewRecorder()

	hub.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var actors []struct {
		ID       string `json:"id"`
		Controls []struct {
			Name string          `json:"name"`
			Kind sup.ControlKind `json:"kind"`
		} `json:"controls"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&actors); err != nil {
		t.Fatal(err)
	}

	if len(actors) != 1 {
		t.Fatalf("expected 1 actor, got %d", len(actors))
	}
	if actors[0].ID != "counter" {
		t.Fatalf("expected actor ID counter, got %q", actors[0].ID)
	}
	if len(actors[0].Controls) != 2 {
		t.Fatalf("expected 2 controls, got %d", len(actors[0].Controls))
	}
}

func TestHubHandleCastControl(t *testing.T) {
	actor := newHubTestActor("counter")
	hub := New("hub")
	hub.RegisterActor(actor)

	req := httptest.NewRequest(http.MethodPost, "/actors/counter/controls/increment", strings.NewReader(`{"amount":5}`))
	rec := httptest.NewRecorder()

	hub.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	select {
	case msg := <-actor.casts.Receive():
		if msg.Amount != 5 {
			t.Fatalf("expected amount 5, got %d", msg.Amount)
		}
	default:
		t.Fatal("expected cast message")
	}
}

func TestHubHandleCallControl(t *testing.T) {
	actor := newHubTestActor("counter")
	hub := New("hub")
	hub.RegisterActor(actor)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := <-actor.calls.Receive()
		req.Reply(hubCount{Count: 42}, nil)
	}()

	req := httptest.NewRequest(http.MethodPost, "/actors/counter/controls/get", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	hub.Handler().ServeHTTP(rec, req)
	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res hubCount
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Count != 42 {
		t.Fatalf("expected count 42, got %d", res.Count)
	}
}

func TestHubObserverRegistersActorAndRecordsEvent(t *testing.T) {
	hub := New("hub")
	supervisor := sup.NewSupervisor("root")
	actor := newHubTestActor("counter")

	hub.Observer().OnActorRegistered(supervisor, actor)

	if _, ok := hub.actor("counter"); !ok {
		t.Fatal("expected actor to be registered")
	}

	events := hub.eventsSnapshot()["counter"]
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != sup.EventActorRegistered {
		t.Fatalf("expected event type %q, got %q", sup.EventActorRegistered, events[0].Type)
	}
	if events[0].SourceID != "counter" {
		t.Fatalf("expected source ID counter, got %q", events[0].SourceID)
	}
}

func TestHubEventsLimitIsPerActor(t *testing.T) {
	hub := New("hub")
	hub.eventLimit = 2

	hub.publish(sup.NewEvent(sup.EventActorStarted, "chatty", nil))
	hub.publish(sup.NewEvent(sup.EventActorStopped, "quiet", nil))
	hub.publish(sup.NewEvent(sup.EventActorRestarting, "chatty", nil))
	hub.publish(sup.NewEvent(sup.EventActorStopped, "chatty", nil))

	snapshot := hub.eventsSnapshot()

	var chatty int
	var quiet int
	for _, events := range snapshot {
		for _, event := range events {
			switch event.SourceID {
			case "chatty":
				chatty++
			case "quiet":
				quiet++
			}
		}
	}

	if chatty != 2 {
		t.Fatalf("expected 2 retained chatty events, got %d", chatty)
	}
	if quiet != 1 {
		t.Fatalf("expected quiet event to be retained, got %d", quiet)
	}
}

func TestHubHandleEvents(t *testing.T) {
	hub := New("hub")
	hub.publish(sup.NewEvent(sup.EventActorStarted, "counter", nil))

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()

	hub.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var events map[string][]sup.Event
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	counterEvents := events["counter"]
	if len(counterEvents) != 1 {
		t.Fatalf("expected 1 counter event, got %d", len(counterEvents))
	}
	if counterEvents[0].Type != sup.EventActorStarted {
		t.Fatalf("expected event type %q, got %q", sup.EventActorStarted, counterEvents[0].Type)
	}
}

func TestHubHandleSignals(t *testing.T) {
	signal := sup.NewSignal("status", "offline")
	hub := New("hub", WithSignal(signal))

	req := httptest.NewRequest(http.MethodGet, "/signals", nil)
	rec := httptest.NewRecorder()

	hub.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var signals []hubSignal
	if err := json.NewDecoder(rec.Body).Decode(&signals); err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].ID != "status" {
		t.Fatalf("expected signal ID status, got %q", signals[0].ID)
	}
	if signals[0].Type != hubSignalValueTypeString {
		t.Fatalf("expected signal type %q, got %q", hubSignalValueTypeString, signals[0].Type)
	}
	if signals[0].Value != "offline" {
		t.Fatalf("expected value offline, got %v", signals[0].Value)
	}
}

func TestHubSignalUpdatesValueAndPublishesEvent(t *testing.T) {
	signal := sup.NewSignal("status", "offline").InitialNotify()
	hub := New("hub", WithSignal(signal))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- hub.Run(ctx)
	}()

	waitFor(t, func() bool {
		signal, ok := hub.signal("status")
		return ok && signal.Value == "offline" && hasHubEvent(hub, sup.EventSignalUpdated, "status")
	})

	if err := signal.Write(context.Background(), "online"); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		signal, ok := hub.signal("status")
		return ok && signal.Value == "online" && hasHubEvent(hub, sup.EventSignalUpdated, "status")
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("hub did not stop")
	}
}

func hasHubEvent(hub *Hub, eventType sup.EventType, sourceID string) bool {
	for _, events := range hub.eventsSnapshot() {
		for _, event := range events {
			if event.Type == eventType && event.SourceID == sourceID {
				return true
			}
		}
	}
	return false
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
