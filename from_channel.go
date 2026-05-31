package sup

import "context"

// FromChannel returns a source that emits values received from ch.
func FromChannel[V any](ch <-chan V) Source[V] {
	return SourceFunc[V](func(ctx context.Context, emit EmitFunc[V]) error {
		for {
			select {
			case <-ctx.Done():
				return nil

			case value, ok := <-ch:
				if !ok {
					return nil
				}

				if err := emit(ctx, value); err != nil {
					return err
				}
			}
		}
	})
}
