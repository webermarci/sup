package rx

// Map transforms each input value. Its capacity-one output coalesces pending
// values to the latest and closes when input closes.
func Map[A, B any](input <-chan A, transform func(A) B) <-chan B {
	output := make(chan B, 1)

	go func() {
		defer close(output)
		for value := range input {
			sendLatest(output, transform(value))
		}
	}()

	return output
}
