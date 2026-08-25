package rx

// Transition describes a change from one value to another.
type Transition[V any] struct {
	From V
	To   V
}

// Transitions emits transitions between consecutive distinct comparable
// values. The first input value establishes the initial state and is not
// emitted.
func Transitions[V comparable](input <-chan V) <-chan Transition[V] {
	return TransitionsFunc(input, func(left, right V) bool {
		return left == right
	})
}

// TransitionsFunc emits transitions between values that equal reports as
// different. The first input value establishes the initial state and is not
// emitted. Each transition starts at the last value accepted as distinct.
func TransitionsFunc[V any](
	input <-chan V,
	equal func(V, V) bool,
) <-chan Transition[V] {
	output := make(chan Transition[V], 1)

	go func() {
		defer close(output)

		var previous V
		initialized := false
		for value := range input {
			if !initialized {
				previous = value
				initialized = true
				continue
			}
			if equal(previous, value) {
				continue
			}

			sendLatest(output, Transition[V]{From: previous, To: value})
			previous = value
		}
	}()

	return output
}
