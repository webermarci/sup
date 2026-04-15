package sup_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestMailbox_TryCastAndClose(t *testing.T) {
	mb := sup.NewMailbox[int](2)

	if mb.Len() != 0 {
		t.Fatalf("expected mailbox length 0, got %d", mb.Len())
	}

	if mb.Cap() != 2 {
		t.Fatalf("expected mailbox capacity 2, got %d", mb.Cap())
	}

	if mb.IsClosed() {
		t.Fatal("expected mailbox to be open")
	}

	if err := mb.TryCast(1); err != nil {
		t.Fatalf("expected TryCast to succeed, got %v", err)
	}

	if err := mb.TryCast(2); err != nil {
		t.Fatalf("expected TryCast to succeed, got %v", err)
	}

	if err := mb.TryCast(3); !errors.Is(err, sup.ErrMailboxFull) {
		t.Fatalf("expected ErrMailboxFull, got %v", err)
	}

	if val := <-mb.Receive(); val != 1 {
		t.Fatalf("expected 1, got %d", val)
	}

	mb.Close()

	if !mb.IsClosed() {
		t.Fatal("expected mailbox to be closed")
	}

	if err := mb.TryCast(4); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed, got %v", err)
	}

	if val := <-mb.Receive(); val != 2 {
		t.Fatalf("expected 2, got %d", val)
	}

	if _, ok := <-mb.Receive(); ok {
		t.Fatal("expected channel to be closed")
	}
}

func TestMailbox_CastContext_Timeout(t *testing.T) {
	mb := sup.NewMailbox[int](0)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := mb.CastContext(ctx, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestMailbox_Cast_BlocksUntilReceiverReady(t *testing.T) {
	mb := sup.NewMailbox[int](0)

	done := make(chan error, 1)

	go func() {
		done <- mb.Cast(42)
	}()

	select {
	case err := <-done:
		t.Fatalf("cast should block, returned early with %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case v := <-mb.Receive():
		if v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for message")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected Cast to succeed, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cast did not complete after receiver read")
	}
}

func TestMailbox_Cast_OnClosedMailbox(t *testing.T) {
	mb := sup.NewMailbox[int](1)
	mb.Close()

	if err := mb.Cast(1); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed from Cast, got %v", err)
	}

	if err := mb.CastContext(t.Context(), 1); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed from CastContext, got %v", err)
	}
}

func TestMailbox_Close_Idempotent(t *testing.T) {
	mb := sup.NewMailbox[int](1)
	mb.Close()
	mb.Close() // must not panic
}

func TestMailbox_Len(t *testing.T) {
	mb := sup.NewMailbox[int](3)

	_ = mb.TryCast(1)
	_ = mb.TryCast(2)

	if mb.Len() != 2 {
		t.Fatalf("expected Len 2, got %d", mb.Len())
	}

	<-mb.Receive()

	if mb.Len() != 1 {
		t.Fatalf("expected Len 1, got %d", mb.Len())
	}
}

type MathReq struct{ A, B int }

type MathActor struct {
	*sup.Mailbox[any]
}

func (a *MathActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-a.Receive():
			if !ok {
				return nil
			}

			switch m := msg.(type) {
			case sup.Request[MathReq, int]:
				if m.Msg.B == 0 {
					m.Reply(0, errors.New("division by zero"))
					continue
				}
				m.Reply(m.Msg.A/m.Msg.B, nil)
			}
		}
	}
}

func TestCall_SuccessAndError(t *testing.T) {
	ctx := t.Context()

	actor := &MathActor{Mailbox: sup.NewMailbox[any](10)}
	go actor.Run(ctx)

	res, err := sup.Call[MathReq, int](actor.Mailbox, MathReq{10, 2})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res != 5 {
		t.Fatalf("expected 5, got %d", res)
	}

	_, err = sup.Call[MathReq, int](actor.Mailbox, MathReq{10, 0})
	if err == nil || err.Error() != "division by zero" {
		t.Fatalf("expected division by zero error, got %v", err)
	}
}

func TestCallContext_TimeoutWhileEnqueueing(t *testing.T) {
	mb := sup.NewMailbox[any](0)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	_, err := sup.CallContext[MathReq, int](ctx, mb, MathReq{1, 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

type NoReplyActor struct {
	*sup.Mailbox[any]
}

func (a *NoReplyActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-a.Receive():
			if !ok {
				return nil
			}

			switch msg.(type) {
			case sup.Request[MathReq, int]:
			}
		}
	}
}

func TestCallContext_TimeoutWaitingForReply(t *testing.T) {
	ctx := t.Context()

	actor := &NoReplyActor{Mailbox: sup.NewMailbox[any](10)}
	go actor.Run(ctx)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	_, err := sup.CallContext[MathReq, int](timeoutCtx, actor.Mailbox, MathReq{10, 2})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestTryCall_MailboxFull(t *testing.T) {
	mb := sup.NewMailbox[any](0)

	_, err := sup.TryCall[MathReq, int](mb, MathReq{1, 1})
	if !errors.Is(err, sup.ErrMailboxFull) {
		t.Fatalf("expected ErrMailboxFull, got %v", err)
	}
}

func TestTryCall_Success(t *testing.T) {
	ctx := t.Context()

	actor := &MathActor{Mailbox: sup.NewMailbox[any](10)}
	go actor.Run(ctx)

	res, err := sup.TryCall[MathReq, int](actor.Mailbox, MathReq{12, 3})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res != 4 {
		t.Fatalf("expected 4, got %d", res)
	}
}

type DelayedReplyActor struct {
	*sup.Mailbox[any]
	count atomic.Int32
}

func (a *DelayedReplyActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-a.Receive():
			if !ok {
				return nil
			}

			switch m := msg.(type) {
			case sup.Request[int, int]:
				n := a.count.Add(1)
				if n == 1 {
					time.Sleep(30 * time.Millisecond)
					m.Reply(111, nil)
					continue
				}
				m.Reply(222, nil)
			}
		}
	}
}

func TestCallContext_LateReplyDoesNotCorruptNextCall(t *testing.T) {
	ctx := t.Context()

	actor := &DelayedReplyActor{Mailbox: sup.NewMailbox[any](10)}
	go actor.Run(ctx)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	_, err := sup.CallContext[int, int](timeoutCtx, actor.Mailbox, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	time.Sleep(40 * time.Millisecond)

	res, err := sup.Call[int, int](actor.Mailbox, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res != 222 {
		t.Fatalf("expected 222, got %d", res)
	}
}

func TestSupervisor_Temporary(t *testing.T) {
	ctx := t.Context()

	var runs atomic.Int32

	actorFn := func(ctx context.Context) error {
		runs.Add(1)
		panic("fatal error")
	}

	supervisor := &sup.Supervisor{
		Policy: sup.Temporary,
	}

	if supervisor.Running() != 0 {
		t.Fatalf("expected 0 running actors, got %d", supervisor.Running())
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait()

	if supervisor.Running() != 0 {
		t.Fatalf("expected 0 running actors after wait, got %d", supervisor.Running())
	}

	if runs.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_Transient(t *testing.T) {
	ctx := t.Context()

	var runs atomic.Int32

	actorFn := func(ctx context.Context) error {
		count := runs.Add(1)
		if count == 1 {
			return errors.New("abnormal exit")
		}
		return nil
	}

	supervisor := &sup.Supervisor{
		Policy:       sup.Transient,
		RestartDelay: 5 * time.Millisecond,
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait()

	if runs.Load() != 2 {
		t.Fatalf("expected 2 runs, got %d", runs.Load())
	}
}

func TestSupervisor_PermanentAndPanicRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var runs atomic.Int32

	actorFn := func(actorCtx context.Context) error {
		count := runs.Add(1)
		if count < 3 {
			panic("simulated panic")
		}

		<-actorCtx.Done()
		return actorCtx.Err()
	}

	supervisor := &sup.Supervisor{
		Policy:       sup.Permanent,
		RestartDelay: 5 * time.Millisecond,
	}

	supervisor.Go(ctx, actorFn)

	time.Sleep(30 * time.Millisecond)

	cancel()
	supervisor.Wait()

	if runs.Load() != 3 {
		t.Fatalf("expected 3 runs before shutdown, got %d", runs.Load())
	}
}

func TestSupervisor_OnRestart(t *testing.T) {
	ctx := t.Context()

	var runs, restarts atomic.Int32

	actorFn := func(ctx context.Context) error {
		count := runs.Add(1)
		if count < 3 {
			return errors.New("failure")
		}
		return nil
	}

	supervisor := &sup.Supervisor{
		Policy:       sup.Transient,
		RestartDelay: 5 * time.Millisecond,
		OnRestart:    func() { restarts.Add(1) },
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait()

	if restarts.Load() != 2 {
		t.Fatalf("expected 2 restarts, got %d", restarts.Load())
	}
}

func TestSupervisor_MaxRestarts(t *testing.T) {
	ctx := t.Context()

	var runs, errorCount atomic.Int32
	var maxRestartsReported atomic.Bool

	actorFn := func(ctx context.Context) error {
		runs.Add(1)
		return errors.New("continuous failure")
	}

	supervisor := &sup.Supervisor{
		Policy:        sup.Permanent,
		RestartDelay:  2 * time.Millisecond,
		MaxRestarts:   3,
		RestartWindow: 1 * time.Second,
		OnError: func(err error) {
			errorCount.Add(1)
			if errors.Is(err, sup.ErrMaxRestartsExceeded) {
				maxRestartsReported.Store(true)
			}
		},
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait()

	// 4 runs → 4 individual OnError calls + 1 ErrMaxRestartsExceeded = 5 total
	if runs.Load() != 4 {
		t.Fatalf("expected exactly 4 runs, got %d", runs.Load())
	}

	if errorCount.Load() != 5 {
		t.Fatalf("expected 5 OnError calls, got %d", errorCount.Load())
	}

	if !maxRestartsReported.Load() {
		t.Fatal("expected OnError to be called with ErrMaxRestartsExceeded")
	}
}

func TestSupervisor_OnError_NotCalledOnCleanExit(t *testing.T) {
	ctx := t.Context()

	var called atomic.Bool

	supervisor := &sup.Supervisor{
		Policy:  sup.Transient,
		OnError: func(err error) { called.Store(true) },
	}

	supervisor.Go(ctx, func(ctx context.Context) error { return nil })
	supervisor.Wait()

	if called.Load() {
		t.Fatal("OnError should not be called on clean exit")
	}
}

func TestSupervisor_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(t.Context())

	actorFn := func(actorCtx context.Context) error {
		<-actorCtx.Done()
		return actorCtx.Err()
	}

	supervisor := &sup.Supervisor{Policy: sup.Permanent}

	for range 100 {
		supervisor.Go(ctx, actorFn)
	}

	time.Sleep(10 * time.Millisecond)
	if runtime.NumGoroutine() <= initialGoroutines {
		t.Fatal("expected goroutines to increase")
	}

	cancel()
	supervisor.Wait()

	time.Sleep(10 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	if finalGoroutines > initialGoroutines+5 {
		t.Fatalf("goroutine leak detected! started with %d, ended with %d", initialGoroutines, finalGoroutines)
	}
}

func TestSupervisor_PanicIncludesStackTrace(t *testing.T) {
	ctx := t.Context()

	var capturedErr atomic.Value

	actorFn := func(ctx context.Context) error {
		panic("something exploded")
	}

	supervisor := &sup.Supervisor{
		Policy:  sup.Temporary,
		OnError: func(err error) { capturedErr.Store(err) },
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait()

	err, ok := capturedErr.Load().(error)
	if !ok || err == nil {
		t.Fatal("expected OnError to be called with a non-nil error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "something exploded") {
		t.Fatalf("expected panic value in error, got: %s", msg)
	}

	if !strings.Contains(msg, "goroutine") {
		t.Fatalf("expected stack trace in error, got: %s", msg)
	}
}

func TestSupervisor_Running_ReflectsActiveCount(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ready := make(chan struct{})

	supervisor := &sup.Supervisor{Policy: sup.Permanent}

	for range 3 {
		supervisor.Go(ctx, func(ctx context.Context) error {
			ready <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
	}

	for range 3 {
		<-ready
	}

	if supervisor.Running() != 3 {
		t.Fatalf("expected 3 running, got %d", supervisor.Running())
	}
}

type BenchmarkActor struct {
	*sup.Mailbox[any]
}

func (a *BenchmarkActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-a.Receive():
			if !ok {
				return nil
			}

			switch m := msg.(type) {
			case sup.Request[int, int]:
				m.Reply(m.Msg, nil)
			}
		}
	}
}

func Benchmark_Cast(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = actor.Mailbox.Cast(1)
	}
	b.StopTimer()
}

func Benchmark_Cast_Concurrent(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = actor.Mailbox.Cast(1)
		}
	})
	b.StopTimer()
}

func Benchmark_CastContext(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = actor.Mailbox.CastContext(b.Context(), 1)
	}
	b.StopTimer()
}

func Benchmark_CastContext_Concurrent(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = actor.Mailbox.CastContext(b.Context(), 1)
		}
	})
	b.StopTimer()
}

func Benchmark_CastContext_Expired(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	ctx, cancel := context.WithCancel(b.Context())
	cancel()

	b.ResetTimer()
	for b.Loop() {
		_ = actor.Mailbox.CastContext(ctx, 1)
	}
	b.StopTimer()
}

func Benchmark_TryCast(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](b.N)}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = actor.Mailbox.TryCast(1)
	}
	b.StopTimer()
}

func Benchmark_TryCast_Concurrent(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](b.N)}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = actor.Mailbox.TryCast(1)
		}
	})
	b.StopTimer()
}

func Benchmark_TryCast_Full(b *testing.B) {
	mb := sup.NewMailbox[int](1)
	_ = mb.TryCast(1)

	b.ResetTimer()
	for b.Loop() {
		_ = mb.TryCast(2)
	}
}

func Benchmark_Call(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_, err := sup.Call[int, int](actor.Mailbox, 1)
		if err != nil {
			b.Fatalf("call failed: %v", err)
		}
	}
	b.StopTimer()
}

func Benchmark_Call_Concurrent(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := sup.Call[int, int](actor.Mailbox, 1)
			if err != nil {
				b.Fatalf("call failed: %v", err)
			}
		}
	})
	b.StopTimer()
}

func Benchmark_CallContext(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_, err := sup.CallContext[int, int](b.Context(), actor.Mailbox, 1)
		if err != nil {
			b.Fatalf("call failed: %v", err)
		}
	}
	b.StopTimer()
}

func Benchmark_CallContext_Concurrent(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](1000)}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := sup.CallContext[int, int](b.Context(), actor.Mailbox, 1)
			if err != nil {
				b.Fatalf("call failed: %v", err)
			}
		}
	})
	b.StopTimer()
}

func Benchmark_CallContext_Expired(b *testing.B) {
	mb := sup.NewMailbox[any](0)

	ctx, cancel := context.WithCancel(b.Context())
	cancel()

	b.ResetTimer()
	for b.Loop() {
		_, _ = sup.CallContext[int, int](ctx, mb, 1)
	}
	b.StopTimer()
}

func Benchmark_TryCall(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](b.N)}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_, err := sup.TryCall[int, int](actor.Mailbox, 1)
		if err != nil {
			b.Fatalf("try call failed: %v", err)
		}
	}
	b.StopTimer()
}

func Benchmark_TryCall_Concurrent(b *testing.B) {
	actor := &BenchmarkActor{Mailbox: sup.NewMailbox[any](b.N)}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := sup.TryCall[int, int](actor.Mailbox, 1)
			if err != nil {
				b.Fatalf("try call failed: %v", err)
			}
		}
	})
	b.StopTimer()
}

type PingPongMsg struct {
	Remaining int
	ReplyTo   *sup.Mailbox[PingPongMsg]
	Done      chan struct{}
}

type CastPingActor struct {
	*sup.Mailbox[PingPongMsg]
}

func (p *CastPingActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-p.Receive():
			if !ok {
				return nil
			}
			if msg.Remaining == 0 {
				close(msg.Done)
				return nil
			}
			_ = msg.ReplyTo.Cast(PingPongMsg{
				Remaining: msg.Remaining - 1,
				ReplyTo:   p.Mailbox,
				Done:      msg.Done,
			})
		}
	}
}

type CastContextPingActor struct {
	*sup.Mailbox[PingPongMsg]
}

func (p *CastContextPingActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-p.Receive():
			if !ok {
				return nil
			}
			if msg.Remaining == 0 {
				close(msg.Done)
				return nil
			}
			_ = msg.ReplyTo.CastContext(ctx, PingPongMsg{
				Remaining: msg.Remaining - 1,
				ReplyTo:   p.Mailbox,
				Done:      msg.Done,
			})
		}
	}
}

type TryCastPingActor struct {
	*sup.Mailbox[PingPongMsg]
}

func (p *TryCastPingActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-p.Receive():
			if !ok {
				return nil
			}
			if msg.Remaining == 0 {
				close(msg.Done)
				return nil
			}

			_ = msg.ReplyTo.TryCast(PingPongMsg{
				Remaining: msg.Remaining - 1,
				ReplyTo:   p.Mailbox,
				Done:      msg.Done,
			})
		}
	}
}

func Benchmark_PingPong_Cast(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	actorA := &CastPingActor{Mailbox: sup.NewMailbox[PingPongMsg](1)}
	actorB := &CastPingActor{Mailbox: sup.NewMailbox[PingPongMsg](1)}

	supervisor := &sup.Supervisor{Policy: sup.Temporary}
	supervisor.Go(ctx, actorA.Run)
	supervisor.Go(ctx, actorB.Run)

	done := make(chan struct{})

	b.ResetTimer()
	_ = actorA.Cast(PingPongMsg{
		Remaining: b.N,
		ReplyTo:   actorB.Mailbox,
		Done:      done,
	})

	<-done
	cancel()
	supervisor.Wait()
}

func Benchmark_PingPong_CastContext(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	actorA := &CastContextPingActor{Mailbox: sup.NewMailbox[PingPongMsg](1)}
	actorB := &CastContextPingActor{Mailbox: sup.NewMailbox[PingPongMsg](1)}

	supervisor := &sup.Supervisor{Policy: sup.Temporary}
	supervisor.Go(ctx, actorA.Run)
	supervisor.Go(ctx, actorB.Run)

	done := make(chan struct{})

	b.ResetTimer()
	_ = actorA.CastContext(ctx, PingPongMsg{
		Remaining: b.N,
		ReplyTo:   actorB.Mailbox,
		Done:      done,
	})

	<-done
	cancel()
	supervisor.Wait()
}

func Benchmark_PingPong_TryCast(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	actorA := &TryCastPingActor{Mailbox: sup.NewMailbox[PingPongMsg](1)}
	actorB := &TryCastPingActor{Mailbox: sup.NewMailbox[PingPongMsg](1)}

	supervisor := &sup.Supervisor{Policy: sup.Temporary}
	supervisor.Go(ctx, actorA.Run)
	supervisor.Go(ctx, actorB.Run)

	done := make(chan struct{})

	b.ResetTimer()
	_ = actorA.TryCast(PingPongMsg{
		Remaining: b.N,
		ReplyTo:   actorB.Mailbox,
		Done:      done,
	})

	<-done
	cancel()
	supervisor.Wait()
}

func Benchmark_Supervisor_SpawnAndExit(b *testing.B) {
	ctx := b.Context()
	supervisor := &sup.Supervisor{Policy: sup.Temporary}

	b.ResetTimer()
	for b.Loop() {
		supervisor.Go(ctx, func(c context.Context) error {
			return nil
		})
	}
	b.StopTimer()

	supervisor.Wait()
}
