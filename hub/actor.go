package hub

import "github.com/webermarci/sup"

type hubActor struct {
	ID       string       `json:"id"`
	Spec     sup.Spec     `json:"spec"`
	Controls []hubControl `json:"controls,omitempty"`
}

func describeActor(actor sup.Actor) hubActor {
	return hubActor{
		ID:       actor.ID(),
		Spec:     actor.Inspect(),
		Controls: describeControls(actor),
	}
}
