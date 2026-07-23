package sup

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"
)

// ErrSupervisorRunning is returned when Run is called while the supervisor is
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

// SupervisorOption configures a Supervisor during construction.
type SupervisorOption func(*Supervisor)

// WithActors adds actors that start with the supervisor.
func WithActors(actors ...Actor) SupervisorOption {
	configured := slices.Clone(actors)
	return func(supervisor *Supervisor) {
		supervisor.actors = append(supervisor.actors, configured...)
	}
}

// WithPolicy configures the supervisor restart policy.
func WithPolicy(policy RestartPolicy) SupervisorOption {
	if policy > Temporary {
		panic("sup: invalid restart policy")
	}
	return func(supervisor *Supervisor) {
		supervisor.policy = policy
	}
}

// WithRestartDelay configures the exact delay between actor restart attempts.
func WithRestartDelay(delay time.Duration) SupervisorOption {
	if delay < 0 {
		panic("sup: restart delay must be non-negative")
	}

	return func(supervisor *Supervisor) {
		supervisor.restartDelay = delay
	}
}

// WithRestartLimit limits restarts within a time window.
func WithRestartLimit(maxRestarts int, window time.Duration) SupervisorOption {
	if maxRestarts <= 0 || window <= 0 {
		panic("sup: maxRestarts and window must be positive")
	}

	return func(supervisor *Supervisor) {
		supervisor.maxRestarts = maxRestarts
		supervisor.restartWindow = window
	}
}

// WithEventSink adds an event sink for runtime events from this supervisor and
// its descendant supervisors.
func WithEventSink(sinks ...EventSink) SupervisorOption {
	configured := slices.Clone(sinks)
	return func(supervisor *Supervisor) {
		supervisor.eventSinks = append(supervisor.eventSinks, configured...)
	}
}

// Supervisor runs a fixed set of actors and restarts them according to its
// restart policy. A supervisor is itself an Actor and may be nested.
type Supervisor struct {
	id            string
	actors        []Actor
	policy        RestartPolicy
	restartDelay  time.Duration
	maxRestarts   int
	restartWindow time.Duration
	eventSinks    []EventSink
	running       atomic.Bool
}

// NewSupervisor creates a supervisor with the given id and options.
func NewSupervisor(id string, options ...SupervisorOption) *Supervisor {
	if id == "" {
		panic("sup: supervisor id cannot be empty")
	}
	supervisor := &Supervisor{
		id:           id,
		policy:       Transient,
		restartDelay: time.Second,
	}

	for _, option := range options {
		if option == nil {
			panic("sup: supervisor option cannot be nil")
		}
		option(supervisor)
	}

	actorIDs := make(map[string]struct{}, len(supervisor.actors))
	for _, actor := range supervisor.actors {
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
	for _, sink := range supervisor.eventSinks {
		if sink == nil {
			panic("sup: cannot add nil event sink")
		}
	}

	return supervisor
}

// ID returns the supervisor id.
func (s *Supervisor) ID() string {
	return s.id
}

// Run starts all configured actors and blocks until they have all stopped, a
// restart limit is exceeded, or ctx is canceled. A supervisor may be run again
// after Run returns. Concurrent calls return ErrSupervisorRunning.
func (s *Supervisor) Run(ctx context.Context) error {
	if !s.running.CompareAndSwap(false, true) {
		return ErrSupervisorRunning
	}
	defer s.running.Store(false)

	if len(s.actors) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(withEventSinks(ctx, s.eventSinks))
	defer cancel()

	results := make(chan error, len(s.actors))
	for _, actor := range s.actors {
		go func() {
			results <- s.runActor(runCtx, actor)
		}()
	}

	remaining := len(s.actors)
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

func (s *Supervisor) runActor(ctx context.Context, actor Actor) error {
	s.emit(ctx, EventActorRegistered, actor, nil, 0)
	restarts := 0
	var windowStart time.Time

	for {
		s.emit(ctx, EventActorStarted, actor, nil, 0)
		actorCtx, cancelActor := context.WithCancel(ctx)
		err := executeSafe(actorCtx, actor.Run)
		cancelActor()
		s.emit(ctx, EventActorStopped, actor, err, 0)

		if ctx.Err() != nil || s.policy == Temporary || (s.policy == Transient && err == nil) {
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
		s.emit(ctx, EventActorRestarting, actor, err, restarts)
		if ctx.Err() != nil {
			return nil
		}
		if s.restartDelay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(s.restartDelay):
			}
		}
	}
}

// Inspect returns the supervisor spec.
func (s *Supervisor) Inspect() Spec {
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
