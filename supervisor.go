package sup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"slices"
	"sync"
	"time"
)

type RestartPolicy uint8

const (
	Permanent RestartPolicy = iota // Always restart, even on clean exits
	Transient                      // Restart on errors/panics, but not on clean exits (nil)
	Temporary                      // Never restart
)

// SupervisorObserver allows observing lifecycle events of supervised actors and the supervisor itself.
// This can be used for logging, monitoring, or triggering side effects based on actor behavior.
type SupervisorObserver struct {
	OnActorRegistered    func(actor Actor)
	OnActorStarted       func(actor Actor)
	OnActorStopped       func(actor Actor, err error)
	OnActorRestarting    func(actor Actor, restartCount int, lastErr error)
	OnSupervisorTerminal func(err error)
}

// SupervisorOption configures a Supervisor.
type SupervisorOption func(*Supervisor)

// WithActor adds an actor to be supervised. Can be called multiple times to add multiple actors.
func WithActor(actor Actor) SupervisorOption {
	return func(s *Supervisor) {
		if actor == nil {
			panic("sup: cannot add nil actor")
		}
		s.actors = append(s.actors, actor)
	}
}

// WithActors adds multiple actors to be supervised.
func WithActors(actors ...Actor) SupervisorOption {
	return func(s *Supervisor) {
		for _, actor := range actors {
			if actor == nil {
				panic("sup: cannot add nil actor")
			}
			s.actors = append(s.actors, actor)
		}
	}
}

// WithPolicy sets the restart policy.
func WithPolicy(policy RestartPolicy) SupervisorOption {
	return func(s *Supervisor) {
		s.policy = policy
	}
}

// WithRestartDelay sets the delay between restarts.
func WithRestartDelay(d time.Duration) SupervisorOption {
	return func(s *Supervisor) {
		if d < 0 {
			panic("sup: restart delay must be non-negative")
		}
		s.restartDelay = d
	}
}

// WithRestartLimit sets the maximum number of restarts allowed within a window.
// Both maxRestarts and window must be positive; otherwise NewSupervisor panics.
func WithRestartLimit(maxRestarts int, window time.Duration) SupervisorOption {
	return func(s *Supervisor) {
		if maxRestarts <= 0 || window <= 0 {
			panic("sup: maxRestarts and window must be positive")
		}
		s.maxRestarts = maxRestarts
		s.restartWindow = window
	}
}

// WithOnError sets a callback function that will be called whenever a supervised actor returns an error or panics.
// The callback receives the actor and the error as arguments.
func WithOnError(handler func(actor Actor, err error)) SupervisorOption {
	return func(s *Supervisor) {
		if handler == nil {
			panic("sup: cannot add nil handler")
		}
		s.onError = handler
	}
}

// WithObserver sets a SupervisorObserver to receive lifecycle event notifications for supervised actors and the supervisor itself.
// This allows external monitoring of actor behavior and supervisor actions.
//
// The observers from the parent supervisors are propagated to the child supervisors.
func WithObserver(observer *SupervisorObserver) SupervisorOption {
	return func(s *Supervisor) {
		if observer == nil {
			panic("sup: cannot add nil observer")
		}
		s.observers = append(s.observers, observer)
	}
}

// WithLogger sets a logger for the supervisor.
func WithLogger(logger *slog.Logger) SupervisorOption {
	return func(s *Supervisor) {
		if logger == nil {
			panic("sup: cannot add nil logger")
		}
		s.setLogger(logger)
	}
}

// Supervisor manages the lifecycle of actor Run loops.
type Supervisor struct {
	*BaseActor
	policy             RestartPolicy
	actors             []Actor
	restartDelay       time.Duration
	maxRestarts        int
	restartWindow      time.Duration
	wg                 sync.WaitGroup
	onError            func(actor Actor, err error)
	terminalErr        chan error
	observers          []*SupervisorObserver
	inheritedObservers []*SupervisorObserver
	mu                 sync.RWMutex
}

// NewSupervisor creates a new Supervisor with the given options.
// Panics if the provided options are invalid.
func NewSupervisor(id string, opts ...SupervisorOption) *Supervisor {
	s := &Supervisor{
		BaseActor:    NewBaseActor(id),
		policy:       Transient,
		restartDelay: time.Second,
		terminalErr:  make(chan error, 1),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Spawn starts the given actor under supervision.
// It will be restarted according to the supervisor's policy if it returns an error or panics.
func (s *Supervisor) Spawn(ctx context.Context, actor Actor) {
	if actor == nil {
		panic("sup: cannot spawn nil actor")
	}

	if actor.ID() == "" {
		panic("sup: actor name cannot be empty")
	}

	actor.setLogger(s.Logger())

	if child, ok := actor.(*Supervisor); ok {
		child.inheritObservers(s.allObservers()...)
	}

	s.notifyActorRegistered(actor)

	s.wg.Go(func() {
		var restarts []time.Time
		if s.maxRestarts > 0 {
			restarts = make([]time.Time, s.maxRestarts)
		}
		rIdx := 0
		restartCount := 0

		for {
			name := actor.ID()

			s.Logger().Info("starting child actor",
				slog.String("child", actor.ID()),
			)

			s.notifyActorStarted(actor)
			err := s.executeSafe(ctx, actor.Run)
			s.notifyActorStopped(actor, err)

			if err != nil {
				s.Logger().Error("child actor failed",
					slog.String("child", actor.ID()),
					slog.Any("err", err),
				)
				if s.onError != nil {
					s.onError(actor, err)
				}
			} else {
				s.Logger().Info("child actor exited cleanly",
					slog.String("child", actor.ID()),
				)
			}

			if ctx.Err() != nil {
				s.Logger().Info("supervisor context canceled, stopping child",
					slog.String("child", actor.ID()),
				)
				return
			}

			if s.policy == Temporary || (s.policy == Transient && err == nil) {
				s.Logger().Info("child actor will not be restarted due to policy",
					slog.String("child", actor.ID()),
					slog.Int("policy", int(s.policy)),
				)
				return
			}

			if s.maxRestarts > 0 && s.restartWindow > 0 {
				now := time.Now()
				oldest := restarts[rIdx]

				if !oldest.IsZero() && now.Sub(oldest) <= s.restartWindow {
					escErr := fmt.Errorf("actor %s exceeded %d restarts in %v", name, s.maxRestarts, s.restartWindow)
					s.Logger().Error("supervisor terminal error",
						slog.String("error", escErr.Error()),
						slog.String("child", actor.ID()),
						slog.Int("max_restarts", s.maxRestarts),
						slog.Duration("window", s.restartWindow),
					)
					s.notifySupervisorTerminal(escErr)

					select {
					case s.terminalErr <- escErr:
					default:
					}
					return
				}

				restarts[rIdx] = now
				rIdx = (rIdx + 1) % s.maxRestarts
			}

			restartCount++
			s.Logger().Warn("restarting child actor",
				slog.String("child", actor.ID()),
				slog.Int("restart_count", restartCount),
			)
			s.notifyActorRestarting(actor, restartCount, err)

			delay := s.restartDelay
			if delay > 0 {
				jitter := time.Duration(rand.Int64N(int64(delay) / 10))
				if rand.N(2) == 0 {
					delay += jitter
				} else {
					delay -= jitter
				}

				t := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
			}
		}
	})
}

// Run starts all actors under supervision and blocks until the context is canceled or all actors have stopped.
func (s *Supervisor) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, actor := range s.actors {
		s.Spawn(childCtx, actor)
	}

	allDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(allDone)
	}()

	select {
	case <-ctx.Done():
		s.Logger().Info("supervisor shutting down via context")
		s.wg.Wait()
		s.Logger().Info("supervisor shutdown complete")
		return ctx.Err()
	case err := <-s.terminalErr:
		s.Logger().Error("supervisor shutting down due to terminal error",
			slog.String("err", err.Error()),
		)
		cancel()
		s.wg.Wait()
		return err
	case <-allDone:
		s.Logger().Info("supervisor shutting down as all actors have stopped")
		return nil
	}
}

// Wait blocks until all supervised actors have stopped.
func (s *Supervisor) Wait() {
	s.wg.Wait()
}

// Inspect returns the specification.
func (s *Supervisor) Inspect() Spec {
	return Spec{
		Kind:         "supervisor",
		Dependencies: []string{},
		Metadata: map[string]string{
			"actor_count":    fmt.Sprintf("%d", len(s.actors)),
			"restart_window": s.restartWindow.String(),
			"restart_delay":  s.restartDelay.String(),
			"max_restarts":   fmt.Sprintf("%d", s.maxRestarts),
		},
	}
}

// Children returns a copy of the list of supervised actors.
func (s *Supervisor) Children() []Actor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var children []Actor
	for _, c := range s.actors {
		children = append(children, c)
	}
	return children
}

func (s *Supervisor) executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(
				fmt.Errorf("%v", r),
				fmt.Errorf("%s", debug.Stack()),
			)
		}
	}()
	return fn(ctx)
}

func (s *Supervisor) notifyActorRegistered(actor Actor) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorRegistered != nil {
			obs.OnActorRegistered(actor)
		}
	})
}

func (s *Supervisor) notifyActorStarted(actor Actor) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorStarted != nil {
			obs.OnActorStarted(actor)
		}
	})
}

func (s *Supervisor) notifyActorStopped(actor Actor, err error) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorStopped != nil {
			obs.OnActorStopped(actor, err)
		}
	})
}

func (s *Supervisor) notifyActorRestarting(actor Actor, restartCount int, lastErr error) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorRestarting != nil {
			obs.OnActorRestarting(actor, restartCount, lastErr)
		}
	})
}

func (s *Supervisor) notifySupervisorTerminal(err error) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnSupervisorTerminal != nil {
			obs.OnSupervisorTerminal(err)
		}
	})
}

func (s *Supervisor) notify(fn func(*SupervisorObserver)) {
	s.mu.RLock()
	observers := make([]*SupervisorObserver, 0, len(s.inheritedObservers)+len(s.observers))
	observers = append(observers, s.inheritedObservers...)
	observers = append(observers, s.observers...)
	s.mu.RUnlock()

	for _, obs := range observers {
		if obs == nil {
			continue
		}

		go func(obs *SupervisorObserver) {
			defer func() {
				_ = recover()
			}()
			fn(obs)
		}(obs)
	}
}

func (s *Supervisor) allObservers() []*SupervisorObserver {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.inheritedObservers) == 0 && len(s.observers) == 0 {
		return []*SupervisorObserver{}
	}

	observers := make([]*SupervisorObserver, 0, len(s.inheritedObservers)+len(s.observers))
	observers = append(observers, s.inheritedObservers...)
	observers = append(observers, s.observers...)

	return observers
}

func (s *Supervisor) inheritObservers(observers ...*SupervisorObserver) {
	s.mu.Lock()

	var added []*SupervisorObserver
	for _, obs := range observers {
		if obs == nil {
			continue
		}
		if slices.Contains(s.observers, obs) || slices.Contains(s.inheritedObservers, obs) {
			continue
		}

		s.inheritedObservers = append(s.inheritedObservers, obs)
		added = append(added, obs)
	}

	children := append([]Actor(nil), s.actors...)
	s.mu.Unlock()

	if len(added) == 0 {
		return
	}

	for _, actor := range children {
		if child, ok := actor.(*Supervisor); ok {
			child.inheritObservers(added...)
		}
	}
}
