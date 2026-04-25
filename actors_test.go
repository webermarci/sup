package sup_test

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/webermarci/sup"
)

type MathReq struct{ A, B int }

type MathActor struct {
	*sup.Mailbox
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
			case sup.CallRequest[MathReq, int]:
				if m.Payload().B == 0 {
					m.Reply(0, errors.New("division by zero"))
					continue
				}
				m.Reply(m.Payload().A/m.Payload().B, nil)
			}
		}
	}
}

type NoReplyActor struct {
	*sup.Mailbox
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
			case sup.CallRequest[MathReq, int]:
				// intentionally never replies
			}
		}
	}
}

type DelayedReplyActor struct {
	*sup.Mailbox
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
			case sup.CallRequest[int, int]:
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

type BenchmarkActor struct {
	*sup.Mailbox
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
			case sup.CallRequest[int, int]:
				m.Reply(m.Payload(), nil)
			}
		}
	}
}
