package hub

import (
	"sort"

	"github.com/webermarci/sup"
)

type hubControl struct {
	Name        string          `json:"name"`
	Kind        sup.ControlKind `json:"kind"`
	InputSchema map[string]any  `json:"input_schema"`
}

func describeControls(actor sup.Actor) []hubControl {
	controllable, ok := actor.(sup.Controllable)
	if !ok {
		return nil
	}

	controls := controllable.Controls()
	res := make([]hubControl, 0, len(controls))
	for _, control := range controls {
		res = append(res, hubControl{
			Name:        control.Name(),
			Kind:        control.Kind(),
			InputSchema: control.InputSchema(),
		})
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Name < res[j].Name
	})

	return res
}

func findControl(actor sup.Actor, name string) (sup.Control, bool) {
	controllable, ok := actor.(sup.Controllable)
	if !ok {
		return nil, false
	}

	for _, control := range controllable.Controls() {
		if control.Name() == name {
			return control, true
		}
	}

	return nil, false
}
