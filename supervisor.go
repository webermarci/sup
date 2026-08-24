package sup

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// ErrSupervisorRunning is returned when Run is called while a supervisor is
// already running.
var ErrSupervisorRunning = errors.New("sup: supervisor is already running")

// RestartPolicy controls when a supervisor restarts a stopped actor.
type RestartPolicy uint8

const (
	// Permanent always restarts actors, even after clean exits.
	Permanent RestartPolicy = iota

	// Transient restarts actors after errors or panics, but not after clean exits.
	Transient

	// Temporary never restarts actors.
	Temporary
)

// RestartDelayFunc calculates the delay before an actor restart. The restart
// count starts at one for the first restart. A non-positive duration means no
// delay.
//
// A supervisor may call the function concurrently for different actors, so it
// must be safe for concurrent use.
type RestartDelayFunc func(restartCount int) time.Duration

func (s *Supervisor) runActor(ctx context.Context, actor Actor) error {
	emitSupervisorEvent(ctx, s, EventActorRegistered, actor, nil, 0)
	restarts := 0
	var windowStart time.Time

	for {
		emitSupervisorEvent(ctx, s, EventActorStarted, actor, nil, 0)
		actorCtx, cancelActor := context.WithCancel(ctx)
		err := executeSafe(actorCtx, actor.Run)
		cancelActor()
		emitSupervisorEvent(ctx, s, EventActorStopped, actor, err, 0)

		if ctx.Err() != nil || s.policy == Temporary ||
			(s.policy == Transient && err == nil) {
			return nil
		}

		if s.maxRestarts > 0 {
			now := time.Now()
			if windowStart.IsZero() || now.Sub(windowStart) > s.restartWindow {
				windowStart = now
				restarts = 0
			}
			if restarts >= s.maxRestarts {
				return fmt.Errorf("actor %s exceeded %d restarts in %v",
					actor.ID(), s.maxRestarts, s.restartWindow)
			}
		}

		restarts++
		emitSupervisorEvent(ctx, s, EventActorRestarting, actor, err, restarts)
		if ctx.Err() != nil {
			return nil
		}

		delay := s.restartDelay
		if s.restartDelayFunc != nil {
			delay = s.restartDelayFunc(restarts)
		}
		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
		}
	}
}

// Supervisor runs a fixed set of actors and restarts them according to its
// restart policy. A supervisor is itself an Actor and may be nested.
type Supervisor struct {
	id                  string
	actors              []Actor
	policy              RestartPolicy
	restartDelay        time.Duration
	restartDelayFunc    RestartDelayFunc
	maxRestarts         int
	restartWindow       time.Duration
	eventSinks          []EventSink
	configMu            sync.Mutex
	configurationFrozen bool
	running             atomic.Bool
}

var _ Actor = (*Supervisor)(nil)
var _ Inspectable = (*Supervisor)(nil)

// NewSupervisor creates a supervisor with the given id and restart policy.
// Optional configuration is added with methods before the first Run call.
func NewSupervisor(id string, policy RestartPolicy) *Supervisor {
	if id == "" {
		panic("sup: supervisor id cannot be empty")
	}
	if policy > Temporary {
		panic("sup: invalid restart policy")
	}
	supervisor := &Supervisor{
		id:           id,
		policy:       policy,
		restartDelay: time.Second,
	}
	return supervisor
}

// AddActor adds one actor that starts with the supervisor and returns the
// supervisor for chaining.
func (s *Supervisor) AddActor(actor Actor) *Supervisor {
	return s.AddActors(actor)
}

// AddActors adds actors that start with the supervisor and returns the
// supervisor for chaining.
func (s *Supervisor) AddActors(actors ...Actor) *Supervisor {
	configured := slices.Clone(actors)
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configurationFrozen {
		panic("sup: supervisor configuration is frozen")
	}

	actorIDs := make(map[string]struct{}, len(s.actors)+len(configured))
	for _, actor := range s.actors {
		actorIDs[actor.ID()] = struct{}{}
	}

	for _, actor := range configured {
		if actor == nil {
			panic("sup: cannot add nil actor")
		}

		id := actor.ID()
		if id == "" {
			panic("sup: actor id cannot be empty")
		}
		if _, exists := actorIDs[id]; exists {
			panic("sup: duplicate actor id: " + id)
		}
		actorIDs[id] = struct{}{}
	}
	s.actors = append(s.actors, configured...)
	return s
}

// SetRestartDelay configures the exact delay between actor restart attempts
// and returns the supervisor for chaining.
func (s *Supervisor) SetRestartDelay(delay time.Duration) *Supervisor {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configurationFrozen {
		panic("sup: supervisor configuration is frozen")
	}
	if delay < 0 {
		panic("sup: restart delay must be non-negative")
	}
	s.restartDelay = delay
	s.restartDelayFunc = nil
	return s
}

// SetRestartDelayFunc configures a function that calculates the delay before
// each actor restart. The restart count starts at one for the first restart.
// A non-positive result means that the actor should restart immediately. It
// returns the supervisor for chaining.
func (s *Supervisor) SetRestartDelayFunc(delay RestartDelayFunc) *Supervisor {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configurationFrozen {
		panic("sup: supervisor configuration is frozen")
	}
	if delay == nil {
		panic("sup: restart delay function cannot be nil")
	}
	s.restartDelayFunc = delay
	return s
}

// SetRestartLimit limits restarts within a time window and returns the
// supervisor for chaining.
func (s *Supervisor) SetRestartLimit(maxRestarts int, window time.Duration) *Supervisor {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configurationFrozen {
		panic("sup: supervisor configuration is frozen")
	}
	if maxRestarts <= 0 || window <= 0 {
		panic("sup: maxRestarts and window must be positive")
	}
	s.maxRestarts = maxRestarts
	s.restartWindow = window
	return s
}

// AddEventSink adds an event sink for runtime events from this supervisor and
// its descendants and returns the supervisor for chaining.
func (s *Supervisor) AddEventSink(sink EventSink) *Supervisor {
	return s.AddEventSinks(sink)
}

// AddEventSinks adds event sinks for runtime events from this supervisor and
// its descendants and returns the supervisor for chaining.
func (s *Supervisor) AddEventSinks(sinks ...EventSink) *Supervisor {
	configured := slices.Clone(sinks)
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configurationFrozen {
		panic("sup: supervisor configuration is frozen")
	}
	for _, sink := range configured {
		if sink == nil {
			panic("sup: cannot add nil event sink")
		}
	}
	s.eventSinks = append(s.eventSinks, configured...)
	return s
}

// ID returns the supervisor id.
func (s *Supervisor) ID() string { return s.id }

// Run starts all configured actors and blocks until they have all stopped, a
// restart limit is exceeded, or ctx is canceled. A supervisor may be run again
// after Run returns. Concurrent calls return ErrSupervisorRunning.
func (s *Supervisor) Run(ctx context.Context) error {
	if ctx == nil {
		panic("sup: supervisor context cannot be nil")
	}
	if !s.running.CompareAndSwap(false, true) {
		return ErrSupervisorRunning
	}
	defer s.running.Store(false)

	s.configMu.Lock()
	s.configurationFrozen = true
	actors := slices.Clone(s.actors)
	eventSinks := slices.Clone(s.eventSinks)
	s.configMu.Unlock()

	if len(actors) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(withEventSinks(ctx, eventSinks))
	defer cancel()

	results := make(chan error, len(actors))
	for _, actor := range actors {
		go func() {
			results <- s.runActor(runCtx, actor)
		}()
	}

	remaining := len(actors)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			cancel()
			drain(results, remaining)
			return nil
		case err := <-results:
			remaining--
			if err == nil {
				continue
			}

			cancel()
			drain(results, remaining)
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}

	return nil
}

func drain(results <-chan error, remaining int) {
	for range remaining {
		<-results
	}
}

// Inspect returns the supervisor spec.
func (s *Supervisor) Inspect() Spec {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	return Spec{
		Kind:         "supervisor",
		Dependencies: []string{},
		Metadata: map[string]any{
			"actor_count":    len(s.actors),
			"restart_window": s.restartWindow,
			"restart_delay":  s.restartDelay,
			"max_restarts":   s.maxRestarts,
		},
	}
}

func executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("%v\n%s", value, debug.Stack())
		}
	}()

	if err = fn(ctx); ctx.Err() != nil {
		return nil
	}
	return err
}
