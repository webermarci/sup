package sup

import (
	"context"
	"slices"
	"time"
)

// EventHandler is an optional actor capability for handling runtime events emitted
// by its supervisor tree. Supervisors register the capability when the actor is
// added. HandleEvent may be called concurrently by different actors and must
// return promptly. A panic is recovered so that event handling cannot interrupt
// supervision.
type EventHandler interface {
	HandleEvent(Event)
}

// EventType identifies the kind of event that occurred.
type EventType string

const (
	// EventActorRegistered is emitted when a supervisor begins supervising an
	// actor during a Run attempt.
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

type eventHandlersContextKey struct{}

func withEventHandlers(ctx context.Context, handlers []EventHandler) context.Context {
	if len(handlers) == 0 {
		return ctx
	}

	parent, _ := ctx.Value(eventHandlersContextKey{}).([]EventHandler)
	return context.WithValue(ctx, eventHandlersContextKey{}, slices.Concat(parent, handlers))
}

func emitEvent(ctx context.Context, event Event) {
	event.Time = time.Now()
	handlers, _ := ctx.Value(eventHandlersContextKey{}).([]EventHandler)
	for _, handler := range handlers {
		func() {
			defer func() { _ = recover() }()
			handler.HandleEvent(event)
		}()
	}
}

func emitSupervisorEvent(
	ctx context.Context,
	supervisor *Supervisor,
	eventType EventType,
	actor Actor,
	err error,
	restarts int,
) {
	emitEvent(ctx, Event{
		Type:         eventType,
		Actor:        actor,
		Supervisor:   supervisor,
		Err:          err,
		RestartCount: restarts,
	})
}
