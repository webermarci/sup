package sup

import (
	"context"
	"testing"
)

func BenchmarkPushedSignal_Write(b *testing.B) {
	ctx := b.Context()

	s := NewPushedSignal("bench-pushed", func(_ context.Context, v int) error {
		return nil
	})
	go s.Run(ctx)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		_ = s.Write(ctx, i)
	}
}
