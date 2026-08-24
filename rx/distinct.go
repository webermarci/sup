package rx

// Distinct emits the first input value and values that differ from the
// previously emitted value.
func Distinct[V comparable](input <-chan V) <-chan V {
	return DistinctFunc(input, func(left, right V) bool {
		return left == right
	})
}

// DistinctFunc emits the first input value and values that equal reports as
// different from the previously emitted value.
func DistinctFunc[V any](
	input <-chan V,
	equal func(V, V) bool,
) <-chan V {
	output := make(chan V, 1)

	go func() {
		defer close(output)

		var previous V
		initialized := false
		for value := range input {
			if !initialized || !equal(previous, value) {
				sendLatest(output, value)
				previous = value
				initialized = true
			}
		}
	}()

	return output
}
