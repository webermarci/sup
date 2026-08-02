package hub

import (
	"encoding/json"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/webermarci/sup"
)

// EventSignalUpdated identifies a signal value update in the hub event stream.
const EventSignalUpdated sup.EventType = "signal:updated"

type hubEvent struct {
	ID        string        `json:"id"`
	Timestamp int64         `json:"timestamp"`
	Type      sup.EventType `json:"type"`
	SourceID  string        `json:"source_id"`
	Payload   any           `json:"payload,omitempty"`
}

type actorRegisteredPayload struct {
	SupervisorID string   `json:"supervisor_id"`
	Spec         sup.Spec `json:"spec"`
}

type actorStartedPayload struct {
	SupervisorID string `json:"supervisor_id"`
}

type actorStoppedPayload struct {
	SupervisorID string `json:"supervisor_id"`
	Error        string `json:"error,omitempty"`
}

type actorRestartingPayload struct {
	SupervisorID string `json:"supervisor_id"`
	RestartCount int    `json:"restart_count"`
	LastError    string `json:"last_error,omitempty"`
}

type signalUpdatedPayload struct {
	Value any `json:"value"`
}

var eventIDCounter atomic.Uint64

func newHubEvent(eventType sup.EventType, sourceID string, payload any, eventTime time.Time) hubEvent {
	if eventTime.IsZero() {
		eventTime = time.Now()
	}
	return hubEvent{
		ID:        strconv.FormatUint(eventIDCounter.Add(1), 10),
		Timestamp: eventTime.UnixMilli(),
		Type:      eventType,
		SourceID:  sourceID,
		Payload:   payload,
	}
}

func snapshotHubEvent(event hubEvent) (hubEvent, []byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return hubEvent{}, nil, err
	}

	var snapshot hubEvent
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return hubEvent{}, nil, err
	}
	return snapshot, data, nil
}

func hubEventFromRuntime(event sup.Event) hubEvent {
	var sourceID string
	if event.Actor != nil {
		sourceID = event.Actor.ID()
	}

	var supervisorID string
	if event.Supervisor != nil {
		supervisorID = event.Supervisor.ID()
	}

	var payload any
	switch event.Type {
	case sup.EventActorRegistered:
		payload = actorRegisteredPayload{
			SupervisorID: supervisorID,
			Spec:         inspectActor(event.Actor),
		}
	case sup.EventActorStarted:
		payload = actorStartedPayload{SupervisorID: supervisorID}
	case sup.EventActorStopped:
		payload = actorStoppedPayload{SupervisorID: supervisorID, Error: errorString(event.Err)}
	case sup.EventActorRestarting:
		payload = actorRestartingPayload{
			SupervisorID: supervisorID,
			RestartCount: event.RestartCount,
			LastError:    errorString(event.Err),
		}
	}

	return newHubEvent(event.Type, sourceID, payload, event.Time)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
