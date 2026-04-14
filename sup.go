package sup

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

var (
	ErrActorPaniced = errors.New("actor paniced")
)

func Spawn[I comparable, A any, S any, R any](
	sys *System,
	producer Producer[I, A, S, R],
) Ref[I, A, S, R] {
	initialActor := producer()
	pid := initialActor.PID()

	proc := &process[A, S, R]{
		mailbox: make(chan envelope[A, S, R], 100),
		pool: &sync.Pool{
			New: func() any {
				return &callContext[R]{
					replyCh: make(chan R, 1),
					errCh:   make(chan error, 1),
				}
			},
		},
	}

	sys.registry.Store(pid, proc)

	go func() {
		<-sys.ctx.Done()
		close(proc.mailbox)
	}()

	go supervise(sys, pid, producer, proc)

	return Ref[I, A, S, R]{PID: pid, sys: sys}
}

func supervise[I comparable, A any, S any, R any](
	sys *System,
	pid I,
	producer Producer[I, A, S, R],
	proc *process[A, S, R],
) {
	defer sys.registry.Delete(pid)

	for {
		actor := producer()

		if err := actor.OnInit(sys.ctx); err != nil {
			time.Sleep(time.Second)
			continue
		}

		crashed := processMailbox(sys.ctx, actor, proc.mailbox)

		if !crashed {
			actor.OnTerminate(nil)
			return
		}

		actor.OnTerminate(ErrActorPaniced)
	}
}

func processMailbox[K comparable, C any, Q any, R any](
	ctx context.Context,
	actor Actor[K, C, Q, R],
	mailbox chan envelope[C, Q, R],
) (crashed bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("supervisor caught panic",
				slog.Any("pid", actor.PID()),
				slog.Any("error", r),
			)
			crashed = true
		}
	}()

	for env := range mailbox {
		if env.isCall {
			res, err := actor.ReceiveCall(ctx, env.callMsg)
			env.replyCh <- res
			env.errCh <- err
		} else {
			actor.ReceiveCast(ctx, env.castMsg)
		}

		if actor.IsStopped() {
			return false
		}
	}

	return false
}
