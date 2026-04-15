package sup_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestMailbox_CastAndClose(t *testing.T) {
	mb := sup.NewMailbox[int](2)

	// Test successful casts
	if err := mb.Cast(1); err != nil {
		t.Fatalf("expected Cast to succeed, got %v", err)
	}
	if err := mb.Cast(2); err != nil {
		t.Fatalf("expected Cast to succeed, got %v", err)
	}

	// Test backpressure (mailbox is full)
	if err := mb.Cast(3); !errors.Is(err, sup.ErrMailboxFull) {
		t.Fatalf("expected ErrMailboxFull, got %v", err)
	}

	// Test receiving
	if val := <-mb.Receive(); val != 1 {
		t.Fatalf("expected 1, got %d", val)
	}

	// Test closing
	mb.Close()
	if err := mb.Cast(4); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed, got %v", err)
	}

	// Channel should be cleanly closed (draining still works)
	if val := <-mb.Receive(); val != 2 {
		t.Fatalf("expected 2, got %d", val)
	}
	if _, ok := <-mb.Receive(); ok {
		t.Fatal("expected channel to be closed")
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
		case msg := <-a.Receive():
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

	// Start actor manually for this simple test
	go actor.Run(ctx)

	// Test successful call
	res, err := sup.Call[MathReq, int](ctx, actor.Mailbox, MathReq{10, 2})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != 5 {
		t.Fatalf("expected 5, got %d", res)
	}

	// Test domain error
	_, err = sup.Call[MathReq, int](ctx, actor.Mailbox, MathReq{10, 0})
	if err == nil || err.Error() != "division by zero" {
		t.Fatalf("expected division by zero error, got %v", err)
	}
}

func TestCall_MailboxFull(t *testing.T) {
	ctx := t.Context()

	// Mailbox with size 0 ensures the Cast blocks/fails instantly if not read
	mb := sup.NewMailbox[any](0)

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer timeoutCancel()

	// Call will fail instantly because mailbox is full/unbuffered
	_, err := sup.Call[MathReq, int](timeoutCtx, mb, MathReq{1, 1})
	if !errors.Is(err, sup.ErrMailboxFull) {
		t.Fatalf("expected ErrMailboxFull, got %v", err)
	}
}

func TestSupervisor_Temporary(t *testing.T) {
	ctx := t.Context()

	var runs atomic.Int32

	actorFn := func(ctx context.Context) error {
		runs.Add(1)
		panic("fatal error") // Crash immediately
	}

	supervisor := &sup.Supervisor{
		Policy: sup.Temporary, // Should NEVER restart
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait() // Will unblock because actor dies and is not restarted

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
			return errors.New("abnormal exit") // Should restart
		}
		return nil // Clean exit, should NOT restart
	}

	supervisor := &sup.Supervisor{
		Policy:       sup.Transient,
		RestartDelay: 5 * time.Millisecond,
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait() // Unblocks after the clean exit

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
			panic("simulated panic") // Should recover and restart
		}

		// 3rd run stays alive until context is canceled
		<-actorCtx.Done()
		return actorCtx.Err()
	}

	supervisor := &sup.Supervisor{
		Policy:       sup.Permanent,
		RestartDelay: 5 * time.Millisecond,
	}

	supervisor.Go(ctx, actorFn)

	// Wait a moment for restarts to happen
	time.Sleep(30 * time.Millisecond)

	// Shut down the system
	cancel()
	supervisor.Wait()

	if runs.Load() != 3 {
		t.Fatalf("expected 3 runs before shutdown, got %d", runs.Load())
	}
}

func TestSupervisor_MaxRestarts(t *testing.T) {
	ctx := t.Context()

	var runs atomic.Int32
	var errReported atomic.Bool

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
			errReported.Store(true)
		},
	}

	supervisor.Go(ctx, actorFn)
	supervisor.Wait() // Should unblock when MaxRestarts is hit

	// Initial boot + 3 restarts = 4 runs total
	if runs.Load() != 4 {
		t.Fatalf("expected exactly 4 runs, got %d", runs.Load())
	}

	if !errReported.Load() {
		t.Fatal("expected OnError to be called after max restarts")
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
		t.Fatalf("goroutine leak detected! Started with %d, ended with %d", initialGoroutines, finalGoroutines)
	}
}

func BenchmarkMailbox_Cast(b *testing.B) {
	mb := sup.NewMailbox[int](b.N + 1)
	b.ResetTimer()
	for b.Loop() {
		_ = mb.Cast(1)
	}
}

func BenchmarkMailbox_ConcurrentCast(b *testing.B) {
	mb := sup.NewMailbox[int](b.N + 1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mb.Cast(1)
		}
	})
}

type PingMsg struct {
	Remaining int
	ReplyTo   *sup.Mailbox[PingMsg]
	Done      chan struct{}
}

type AsyncPingActor struct {
	*sup.Mailbox[PingMsg]
}

func (p *AsyncPingActor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-p.Receive():
			if msg.Remaining == 0 {
				close(msg.Done)
				return nil
			}

			_ = msg.ReplyTo.Cast(PingMsg{
				Remaining: msg.Remaining - 1,
				ReplyTo:   p.Mailbox,
				Done:      msg.Done,
			})
		}
	}
}

func BenchmarkActor_PingPongCast(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	actorA := &AsyncPingActor{Mailbox: sup.NewMailbox[PingMsg](1)}
	actorB := &AsyncPingActor{Mailbox: sup.NewMailbox[PingMsg](1)}

	supervisor := &sup.Supervisor{Policy: sup.Temporary}
	supervisor.Go(ctx, actorA.Run)
	supervisor.Go(ctx, actorB.Run)

	done := make(chan struct{})

	b.ResetTimer()

	_ = actorA.Cast(PingMsg{
		Remaining: b.N,
		ReplyTo:   actorB.Mailbox,
		Done:      done,
	})

	<-done

	cancel()
	supervisor.Wait()
}
