package sup

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"slices"
	"sync"
	"time"
)

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

// SupervisorObserver receives lifecycle callbacks from a supervisor.
type SupervisorObserver struct {
	OnActorRegistered    func(supervisor *Supervisor, actor Actor)
	OnActorStarted       func(supervisor *Supervisor, actor Actor)
	OnActorStopped       func(supervisor *Supervisor, actor Actor, err error)
	OnActorRestarting    func(supervisor *Supervisor, actor Actor, restartCount int, lastErr error)
	OnSupervisorTerminal func(supervisor *Supervisor, err error)
}

// Supervisor runs actors and restarts them according to its restart policy.
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

// NewSupervisor creates a supervisor with the given id.
func NewSupervisor(id string) *Supervisor {
	return &Supervisor{
		BaseActor:    NewBaseActor(id),
		policy:       Transient,
		restartDelay: time.Second,
		terminalErr:  make(chan error, 1),
	}
}

// Actor adds an actor to be started when the supervisor runs.
func (s *Supervisor) Actor(actor Actor) *Supervisor {
	if actor == nil {
		panic("sup: cannot add nil actor")
	}

	s.mu.Lock()
	s.actors = append(s.actors, actor)
	s.mu.Unlock()

	return s
}

// Actors adds multiple actors to be started when the supervisor runs.
func (s *Supervisor) Actors(actors ...Actor) *Supervisor {
	for _, actor := range actors {
		s.Actor(actor)
	}

	return s
}

// Policy sets the supervisor restart policy.
func (s *Supervisor) Policy(policy RestartPolicy) *Supervisor {
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()

	return s
}

// RestartDelay sets the delay between actor restart attempts.
func (s *Supervisor) RestartDelay(d time.Duration) *Supervisor {
	if d < 0 {
		panic("sup: restart delay must be non-negative")
	}

	s.mu.Lock()
	s.restartDelay = d
	s.mu.Unlock()

	return s
}

// RestartLimit sets the maximum restarts allowed within a time window.
func (s *Supervisor) RestartLimit(maxRestarts int, window time.Duration) *Supervisor {
	if maxRestarts <= 0 || window <= 0 {
		panic("sup: maxRestarts and window must be positive")
	}

	s.mu.Lock()
	s.maxRestarts = maxRestarts
	s.restartWindow = window
	s.mu.Unlock()

	return s
}

// OnError sets a handler called when an actor exits with an error or panic.
func (s *Supervisor) OnError(handler func(actor Actor, err error)) *Supervisor {
	if handler == nil {
		panic("sup: cannot add nil handler")
	}

	s.mu.Lock()
	s.onError = handler
	s.mu.Unlock()

	return s
}

// Observer adds a lifecycle observer to the supervisor.
func (s *Supervisor) Observer(observer *SupervisorObserver) *Supervisor {
	if observer == nil {
		panic("sup: cannot add nil observer")
	}

	s.mu.Lock()
	s.observers = append(s.observers, observer)
	s.mu.Unlock()

	return s
}

// Spawn starts and supervises an actor immediately.
func (s *Supervisor) Spawn(ctx context.Context, actor Actor) {
	if actor == nil {
		panic("sup: cannot spawn nil actor")
	}

	if actor.ID() == "" {
		panic("sup: actor name cannot be empty")
	}

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

			s.notifyActorStarted(actor)
			err := s.executeSafe(ctx, actor.Run)
			s.notifyActorStopped(actor, err)

			if err != nil {
				if s.onError != nil {
					s.onError(actor, err)
				}
			}

			if ctx.Err() != nil {
				return
			}

			if s.policy == Temporary || (s.policy == Transient && err == nil) {
				return
			}

			if s.maxRestarts > 0 && s.restartWindow > 0 {
				now := time.Now()
				oldest := restarts[rIdx]

				if !oldest.IsZero() && now.Sub(oldest) <= s.restartWindow {
					escErr := fmt.Errorf("actor %s exceeded %d restarts in %v",
						name, s.maxRestarts, s.restartWindow)
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
			s.notifyActorRestarting(actor, restartCount, err)

			delay := s.restartDelay
			if delay > 0 {
				jitterRange := int64(delay) / 10
				if jitterRange > 0 {
					jitter := time.Duration(rand.Int64N(jitterRange))
					if rand.N(2) == 0 {
						delay += jitter
					} else {
						delay -= jitter
					}
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

// Run starts configured actors and blocks until they stop, fail, or the context is canceled.
func (s *Supervisor) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.RLock()
	actors := append([]Actor(nil), s.actors...)
	s.mu.RUnlock()

	for _, actor := range actors {
		s.Spawn(childCtx, actor)
	}

	if len(actors) == 0 {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			return ctx.Err()
		case err := <-s.terminalErr:
			cancel()
			s.wg.Wait()
			return err
		}
	}

	allDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(allDone)
	}()

	select {
	case <-ctx.Done():
		s.wg.Wait()
		return ctx.Err()
	case err := <-s.terminalErr:
		cancel()
		s.wg.Wait()
		return err
	case <-allDone:
		return nil
	}
}

// Wait blocks until all spawned actors have stopped.
func (s *Supervisor) Wait() {
	s.wg.Wait()
}

// Inspect returns the supervisor spec.
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

// Children returns a copy of the configured child actors.
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
			obs.OnActorRegistered(s, actor)
		}
	})
}

func (s *Supervisor) notifyActorStarted(actor Actor) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorStarted != nil {
			obs.OnActorStarted(s, actor)
		}
	})
}

func (s *Supervisor) notifyActorStopped(actor Actor, err error) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorStopped != nil {
			obs.OnActorStopped(s, actor, err)
		}
	})
}

func (s *Supervisor) notifyActorRestarting(actor Actor, restartCount int, lastErr error) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnActorRestarting != nil {
			obs.OnActorRestarting(s, actor, restartCount, lastErr)
		}
	})
}

func (s *Supervisor) notifySupervisorTerminal(err error) {
	s.notify(func(obs *SupervisorObserver) {
		if obs.OnSupervisorTerminal != nil {
			obs.OnSupervisorTerminal(s, err)
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
