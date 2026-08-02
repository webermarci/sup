package rx

// Filter emits input values accepted by keep. Its capacity-one output
// coalesces pending values to the latest and closes when input closes.
func Filter[V any](input <-chan V, keep func(V) bool) <-chan V {
	if input == nil {
		panic("rx: filter input cannot be nil")
	}

	if keep == nil {
		panic("rx: filter function cannot be nil")
	}

	output := make(chan V, 1)

	go func() {
		defer close(output)
		for value := range input {
			if keep(value) {
				sendLatest(output, value)
			}
		}
	}()

	return output
}
