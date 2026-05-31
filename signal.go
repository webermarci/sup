package sup

import (
	"context"
	"strconv"
	"time"
)

// ReadableSignal is a signal that can be read, watched, and subscribed to.
type ReadableSignal[V any] interface {
	ReaderSignal[V]
	SubscriberSignal[V]
	WatcherSignal
}

// WritableSignal is a readable signal that can also be written to.
type WritableSignal[V any] interface {
	ReadableSignal[V]
	WriterSignal[V]
}

// ReaderSignal is a signal that exposes its current value.
type ReaderSignal[V any] interface {
	Actor

	// Read returns the current signal value.
	Read() V
}

// WriterSignal is a signal that accepts new values.
type WriterSignal[V any] interface {
	Actor

	// Write updates the signal value.
	Write(context.Context, V) error
}

// WatcherSignal is a signal that notifies watchers when its value changes.
type WatcherSignal interface {
	Actor

	// Watch returns a channel that receives a notification when the signal changes.
	Watch(ctx context.Context) <-chan struct{}
}

// SubscriberSignal is a signal that publishes changed values to subscribers.
type SubscriberSignal[V any] interface {
	Actor

	// Subscribe returns a channel that receives changed signal values.
	Subscribe(context.Context) <-chan V
}

// Signal stores a value and publishes updates to subscribers and watchers.
type Signal[V any] struct {
	*baseSignal[V]
	sources    []Source[V]
	processors []Processor[V]
	errCh      chan error
}

// NewSignal creates a signal with the given id and initial value.
func NewSignal[V any](
	id string,
	initial V,
) *Signal[V] {
	return &Signal[V]{
		baseSignal: newBaseSignal(id, initial),
		errCh:      make(chan error, 1),
	}
}

// InitialNotify makes new subscribers receive the current value immediately.
func (s *Signal[V]) InitialNotify() *Signal[V] {
	s.setInitialNotify(true)
	return s
}

// Buffer sets the subscriber channel buffer size.
func (s *Signal[V]) Buffer(size int) *Signal[V] {
	s.setBroadcasterBuffer(size)
	return s
}

// Equal sets the equality function used to suppress unchanged values.
func (s *Signal[V]) Equal(eq func(a, b V) bool) *Signal[V] {
	s.setEqual(eq)
	return s
}

// Source adds a source that writes values to the signal when it runs.
func (s *Signal[V]) Source(source Source[V]) *Signal[V] {
	s.sources = append(s.sources, source)
	return s
}

// Poll adds a polling source to the signal.
func (s *Signal[V]) Poll(
	interval time.Duration,
	fn func(context.Context) (V, error),
) *Signal[V] {
	return s.Source(Poll(interval, fn))
}

// Processor adds a processor to the signal pipeline.
func (s *Signal[V]) Processor(processor Processor[V]) *Signal[V] {
	s.mu.Lock()
	s.processors = append(s.processors, processor)
	s.mu.Unlock()

	return s
}

// Map adds a processor that transforms each value with fn.
func (s *Signal[V]) Map(fn func(V) V) *Signal[V] {
	return s.Processor(Map(fn))
}

// Filter adds a processor that emits only values accepted by fn.
func (s *Signal[V]) Filter(fn func(V) bool) *Signal[V] {
	return s.Processor(Filter(fn))
}

// Debounce adds a processor that emits the latest value after wait has elapsed.
func (s *Signal[V]) Debounce(wait time.Duration) *Signal[V] {
	return s.Processor(Debounce[V](wait))
}

// Throttle adds a processor that emits at most one value per interval.
func (s *Signal[V]) Throttle(interval time.Duration) *Signal[V] {
	return s.Processor(Throttle[V](interval))
}

// Write updates the signal value after applying configured processors.
func (s *Signal[V]) Write(ctx context.Context, value V) error {
	return s.process(ctx, value)
}

// Effect creates an effect that reacts to this signal.
func (s *Signal[V]) Effect(
	id string,
	fn EffectFunc[V],
) *Effect[V] {
	return NewEffect(id, s, fn)
}

// Run starts the signal sources and blocks until they stop, fail, or the context is canceled.
func (s *Signal[V]) Run(ctx context.Context) error {
	defer s.closeAll()

	s.resetProcessors()

	if len(s.sources) == 0 {
		<-ctx.Done()
		return nil
	}

	sourceCtx, cancelSources := context.WithCancel(ctx)
	defer cancelSources()

	sourceErrCh := make(chan error, len(s.sources))

	for _, source := range s.sources {
		go func() {
			sourceErrCh <- source.Run(sourceCtx, s.Write)
		}()
	}

	runningSources := len(s.sources)

	for {
		select {
		case <-ctx.Done():
			cancelSources()
			return nil

		case err := <-s.errors():
			cancelSources()
			return err

		case err := <-sourceErrCh:
			runningSources--

			if err != nil {
				cancelSources()
				return err
			}

			if runningSources == 0 {
				sourceErrCh = nil
			}
		}
	}
}

// Inspect returns the signal spec.
func (s *Signal[V]) Inspect() Spec {
	s.mu.RLock()
	initialNotify := s.initialNotify
	s.mu.RUnlock()

	return Spec{
		Kind:         "signal",
		Dependencies: nil,
		Metadata: map[string]string{
			"initial_notify": strconv.FormatBool(initialNotify),
		},
	}
}

func (s *Signal[V]) process(ctx context.Context, value V) error {
	s.mu.Lock()
	if len(s.processors) == 0 {
		changed := s.setLocked(value)
		s.mu.Unlock()

		if changed {
			s.broadcaster.notify(value)
		}

		return nil
	}

	if value, emit, handled := s.processBuiltinsLocked(value); handled {
		if !emit {
			s.mu.Unlock()
			return nil
		}

		changed := s.setLocked(value)
		s.mu.Unlock()

		if changed {
			s.broadcaster.notify(value)
		}

		return nil
	}
	s.mu.Unlock()

	emitted, err := s.processCollect(ctx, value)
	if err != nil {
		return err
	}

	for _, v := range emitted {
		s.broadcaster.notify(v)
	}

	return nil
}

func (s *Signal[V]) processBuiltinsLocked(value V) (V, bool, bool) {
	for _, processor := range s.processors {
		switch processor.(type) {
		case mapProcessor[V], filterProcessor[V]:
		default:
			return value, false, false
		}
	}

	for _, processor := range s.processors {
		switch processor := processor.(type) {
		case mapProcessor[V]:
			value = processor.fn(value)

		case filterProcessor[V]:
			if !processor.fn(value) {
				return value, false, true
			}
		}
	}

	return value, true, true
}

func (s *Signal[V]) processCollect(ctx context.Context, value V) ([]V, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var emitted []V

	if len(s.processors) == 0 {
		if s.setLocked(value) {
			emitted = append(emitted, value)
		}

		return emitted, nil
	}

	err := s.processFromLocked(ctx, 0, value, &emitted)
	return emitted, err
}

func (s *Signal[V]) processFromLocked(
	ctx context.Context,
	index int,
	value V,
	emitted *[]V,
) error {
	if index >= len(s.processors) {
		if s.setLocked(value) {
			*emitted = append(*emitted, value)
		}

		return nil
	}

	switch processor := s.processors[index].(type) {
	case mapProcessor[V]:
		return s.processFromLocked(ctx, index+1, processor.fn(value), emitted)

	case filterProcessor[V]:
		if !processor.fn(value) {
			return nil
		}
		return s.processFromLocked(ctx, index+1, value, emitted)
	}

	var emitErr error

	runtime := signalProcessorRuntime[V]{
		ctx:    ctx,
		signal: s,
		index:  index,
		output: emitted,
		err:    &emitErr,
	}

	err := s.processors[index].Process(ctx, value, runtime)
	if err != nil {
		return err
	}

	return emitErr
}

func (s *Signal[V]) processTimer(
	ctx context.Context,
	processorIndex int,
	timerID uint64,
) ([]V, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if processorIndex < 0 || processorIndex >= len(s.processors) {
		return nil, nil
	}

	var emitted []V
	var emitErr error

	runtime := signalProcessorRuntime[V]{
		ctx:    ctx,
		signal: s,
		index:  processorIndex,
		output: &emitted,
		err:    &emitErr,
	}

	err := s.processors[processorIndex].OnTimer(ctx, timerID, runtime)
	if err != nil {
		return nil, err
	}

	return emitted, emitErr
}

func (s *Signal[V]) resetProcessors() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, processor := range s.processors {
		if resettable, ok := processor.(Resettable); ok {
			resettable.Reset()
		}
	}
}

func (s *Signal[V]) reportError(err error) {
	if err == nil || s.errCh == nil {
		return
	}

	select {
	case s.errCh <- err:
	default:
	}
}

func (s *Signal[V]) errors() <-chan error {
	return s.errCh
}

type signalProcessorRuntime[V any] struct {
	ctx    context.Context
	signal *Signal[V]
	index  int
	output *[]V
	err    *error
}

// Emit emits a processed value through the remaining processor pipeline.
func (rt signalProcessorRuntime[V]) Emit(value V) {
	if rt.err != nil && *rt.err != nil {
		return
	}

	err := rt.signal.processFromLocked(rt.ctx, rt.index+1, value, rt.output)
	if err != nil && rt.err != nil {
		*rt.err = err
	}
}

// After schedules a timer callback for the current processor.
func (rt signalProcessorRuntime[V]) After(delay time.Duration, timerID uint64) {
	ctx := rt.ctx
	processorIndex := rt.index

	time.AfterFunc(delay, func() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		emitted, err := rt.signal.processTimer(ctx, processorIndex, timerID)
		if err != nil {
			rt.signal.reportError(err)
			return
		}

		for _, v := range emitted {
			rt.signal.broadcaster.notify(v)
		}
	})
}
