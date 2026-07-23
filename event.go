package sup

import (
	"context"
	"slices"
	"time"
)

// EventSink consumes runtime events emitted by a supervisor tree. HandleEvent may
// be called concurrently by different actors and must return promptly. A panic
// is recovered so that event handling cannot interrupt supervision.
type EventSink interface {
	HandleEvent(Event)
}

// EventSinkFunc adapts a function into an EventSink.
type EventSinkFunc func(Event)

// HandleEvent calls f with event.
func (f EventSinkFunc) HandleEvent(event Event) {
	f(event)
}

// EventType identifies the kind of event that occurred.
type EventType string

const (
	// EventActorRegistered is emitted when an actor is registered with a supervisor.
	EventActorRegistered EventType = "actor:registered"

	// EventActorStarted is emitted when an actor starts running.
	EventActorStarted EventType = "actor:started"

	// EventActorStopped is emitted when an actor stops running.
	EventActorStopped EventType = "actor:stopped"

	// EventActorRestarting is emitted before a supervisor restarts an actor.
	EventActorRestarting EventType = "actor:restarting"
)

// Event describes an actor lifecycle transition emitted by a supervisor.
type Event struct {
	Type         EventType
	Time         time.Time
	Actor        Actor
	Supervisor   *Supervisor
	Err          error
	RestartCount int
}

type eventSinksContextKey struct{}

func withEventSinks(ctx context.Context, sinks []EventSink) context.Context {
	if len(sinks) == 0 {
		return ctx
	}

	parent, _ := ctx.Value(eventSinksContextKey{}).([]EventSink)
	return context.WithValue(ctx, eventSinksContextKey{}, slices.Concat(parent, sinks))
}

func emitEvent(ctx context.Context, event Event) {
	event.Time = time.Now()
	sinks, _ := ctx.Value(eventSinksContextKey{}).([]EventSink)
	for _, sink := range sinks {
		func() {
			defer func() { _ = recover() }()
			sink.HandleEvent(event)
		}()
	}
}

func (s *Supervisor) emit(ctx context.Context, eventType EventType, actor Actor, err error, restarts int) {
	emitEvent(ctx, Event{
		Type:         eventType,
		Actor:        actor,
		Supervisor:   s,
		Err:          err,
		RestartCount: restarts,
	})
}
