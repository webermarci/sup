package sup

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

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

	// EventSupervisorTerminal is emitted when a supervisor reaches a terminal error.
	EventSupervisorTerminal EventType = "supervisor:terminal"

	// EventSignalUpdated is emitted when a signal value changes.
	EventSignalUpdated EventType = "signal:updated"
)

// Event describes an occurrence emitted by an actor, supervisor, or signal.
type Event struct {
	ID        string    `json:"id"`
	Timestamp int64     `json:"timestamp"`
	Type      EventType `json:"type"`
	SourceID  string    `json:"source_id"`
	Payload   any       `json:"payload,omitempty"`
}

// ActorRegisteredPayload is the payload for EventActorRegistered.
type ActorRegisteredPayload struct {
	SupervisorID string `json:"supervisor_id"`
}

// ActorStartedPayload is the payload for EventActorStarted.
type ActorStartedPayload struct {
	SupervisorID string `json:"supervisor_id"`
}

// ActorStoppedPayload is the payload for EventActorStopped.
type ActorStoppedPayload struct {
	SupervisorID string `json:"supervisor_id"`
	Error        string `json:"error,omitempty"`
}

// ActorRestartingPayload is the payload for EventActorRestarting.
type ActorRestartingPayload struct {
	SupervisorID string `json:"supervisor_id"`
	RestartCount int    `json:"restart_count"`
	LastError    string `json:"last_error,omitempty"`
}

// SupervisorTerminalPayload is the payload for EventSupervisorTerminal.
type SupervisorTerminalPayload struct {
	Error string `json:"error,omitempty"`
}

// SignalUpdatedPayload is the payload for EventSignalUpdated.
type SignalUpdatedPayload struct {
	Value any `json:"value"`
}

// NewEvent creates an event with a generated id and current timestamp.
func NewEvent(eventType EventType, sourceID string, payload any) Event {
	return Event{
		ID:        newEventID(),
		Timestamp: time.Now().UnixMilli(),
		Type:      eventType,
		SourceID:  sourceID,
		Payload:   payload,
	}
}

func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	return hex.EncodeToString(b[:])
}
