package sup_test

import (
	"context"
	"testing"

	"github.com/webermarci/sup"
)

type benchmarkActor struct {
	*sup.BaseActor
	*sup.Mailbox
}

func (a *benchmarkActor) Run(ctx context.Context) error {
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

func Benchmark_Cast(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = sup.Cast(actor.Mailbox, 1)
	}
}

func Benchmark_Cast_Concurrent(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sup.Cast(actor.Mailbox, 1)
		}
	})
}

func Benchmark_CastContext(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = sup.CastContext(b.Context(), actor.Mailbox, 1)
	}
}

func Benchmark_CastContext_Concurrent(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sup.CastContext(b.Context(), actor.Mailbox, 1)
		}
	})
}

func Benchmark_CastContext_Expired(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	ctx, cancel := context.WithCancel(b.Context())
	cancel()

	b.ResetTimer()
	for b.Loop() {
		_ = sup.CastContext(ctx, actor.Mailbox, 1)
	}
}

func Benchmark_TryCast(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(b.N),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = sup.TryCast(actor.Mailbox, 1)
	}
}

func Benchmark_TryCast_Concurrent(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(b.N),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sup.TryCast(actor.Mailbox, 1)
		}
	})
}

func Benchmark_TryCast_Full(b *testing.B) {
	mb := sup.NewMailbox(1)
	_ = sup.TryCast(mb, 1)

	b.ResetTimer()
	for b.Loop() {
		_ = sup.TryCast(mb, 1)
	}
}

func Benchmark_Call(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		if _, err := sup.Call[int, int](actor.Mailbox, 1); err != nil {
			b.Fatalf("call failed: %v", err)
		}
	}
}

func Benchmark_Call_Concurrent(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := sup.Call[int, int](actor.Mailbox, 1); err != nil {
				b.Fatalf("call failed: %v", err)
			}
		}
	})
}

func Benchmark_CallContext(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		if _, err := sup.CallContext[int, int](b.Context(), actor.Mailbox, 1); err != nil {
			b.Fatalf("call failed: %v", err)
		}
	}
}

func Benchmark_CallContext_Concurrent(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(1000),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := sup.CallContext[int, int](b.Context(), actor.Mailbox, 1); err != nil {
				b.Fatalf("call failed: %v", err)
			}
		}
	})
}

func Benchmark_CallContext_Expired(b *testing.B) {
	mb := sup.NewMailbox(0)

	ctx, cancel := context.WithCancel(b.Context())
	cancel()

	b.ResetTimer()
	for b.Loop() {
		_, _ = sup.CallContext[int, int](ctx, mb, 1)
	}
}

func Benchmark_TryCall(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(b.N),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		if _, err := sup.TryCall[int, int](actor.Mailbox, 1); err != nil {
			b.Fatalf("try call failed: %v", err)
		}
	}
}

func Benchmark_TryCall_Concurrent(b *testing.B) {
	actor := &benchmarkActor{
		BaseActor: sup.NewBaseActor(b.Name()),
		Mailbox:   sup.NewMailbox(b.N),
	}
	go actor.Run(b.Context())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := sup.TryCall[int, int](actor.Mailbox, 1); err != nil {
				b.Fatalf("try call failed: %v", err)
			}
		}
	})
}

func Benchmark_Supervisor_SpawnAndExit(b *testing.B) {
	actor := sup.ActorFunc(b.Name(), func(ctx context.Context) error {
		return nil
	})
	supervisor := sup.NewSupervisor(
		b.Name(),
		sup.WithPolicy(sup.Temporary),
	)
	go supervisor.Run(b.Context())

	b.ResetTimer()
	for b.Loop() {
		supervisor.Spawn(b.Context(), actor)
	}
	b.StopTimer()
}
