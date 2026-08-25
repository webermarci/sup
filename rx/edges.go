package rx

// Edge describes the direction of a boolean transition.
type Edge string

const (
	// EdgeRising is a transition from false to true.
	EdgeRising Edge = "rising"
	// EdgeFalling is a transition from true to false.
	EdgeFalling Edge = "falling"
)

// Edges emits the direction of each boolean transition. The first input value
// establishes the initial state and is not emitted.
func Edges(input <-chan bool) <-chan Edge {
	return Map(Transitions(input), func(transition Transition[bool]) Edge {
		if transition.To {
			return EdgeRising
		}
		return EdgeFalling
	})
}
