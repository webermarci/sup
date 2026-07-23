package hub

import "github.com/webermarci/sup"

type hubActor struct {
	ID   string   `json:"id"`
	Spec sup.Spec `json:"spec"`
}

func describeActor(actor sup.Actor) hubActor {
	return hubActor{
		ID:   actor.ID(),
		Spec: inspectActor(actor),
	}
}

func inspectActor(actor sup.Actor) sup.Spec {
	if inspectable, ok := actor.(sup.Inspectable); ok {
		return normalizeSpec(inspectable.Inspect(), "actor")
	}
	return normalizeSpec(sup.Spec{}, "actor")
}

func normalizeSpec(spec sup.Spec, fallbackKind string) sup.Spec {
	if spec.Kind == "" {
		spec.Kind = fallbackKind
	}
	if spec.Dependencies == nil {
		spec.Dependencies = []string{}
	}
	if spec.Metadata == nil {
		spec.Metadata = map[string]any{}
	}
	return spec
}
