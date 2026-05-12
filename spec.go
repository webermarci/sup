package sup

// Inspectable is an interface for components that can provide a structured
// specification describing their type, dependencies, and configuration.
// Implement this interface to enable introspection, visualization, or debugging
// of actor system components.
type Inspectable interface {
	Inspect() Spec
}

// Spec represents a structured description of an inspectable component.
// It captures the component's kind, its dependency names, and arbitrary
// configuration metadata for visualization and debugging purposes.
type Spec struct {
	Kind         string            `json:"kind"`
	Dependencies []string          `json:"dependencies"`
	Metadata     map[string]string `json:"metadata"`
}
