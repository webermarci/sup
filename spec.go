package sup

// Spec represents a structured description of an inspectable component.
// It captures the component's kind, its dependency names, and arbitrary
// configuration metadata for visualization and debugging purposes.
type Spec struct {
	Kind         string         `json:"kind"`
	Dependencies []string       `json:"dependencies"`
	Metadata     map[string]any `json:"metadata"`
}
