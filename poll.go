package sup

import (
	"context"
	"time"
)

// Poll returns a source that calls fn on each interval and emits the result.
func Poll[V any](
	interval time.Duration,
	fn func(context.Context) (V, error),
) Source[V] {
	return SourceFunc[V](func(ctx context.Context, emit EmitFunc[V]) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil

			case <-ticker.C:
				value, err := fn(ctx)
				if err != nil {
					return err
				}

				if err := emit(ctx, value); err != nil {
					return err
				}
			}
		}
	})
}
