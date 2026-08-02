package rx

// Map transforms each input value. Its capacity-one output coalesces pending
// values to the latest and closes when input closes.
func Map[A, B any](input <-chan A, transform func(A) B) <-chan B {
	if input == nil {
		panic("rx: map input cannot be nil")
	}

	if transform == nil {
		panic("rx: map function cannot be nil")
	}

	output := make(chan B, 1)

	go func() {
		defer close(output)
		for value := range input {
			sendLatest(output, transform(value))
		}
	}()

	return output
}
