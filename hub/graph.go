package hub

import (
	"cmp"
	"maps"
	"slices"

	"github.com/webermarci/sup"
)

type graphNode struct {
	ID   string   `json:"id"`
	Spec sup.Spec `json:"spec"`
}

type graphEdge struct {
	SupervisorID string `json:"supervisor_id"`
	ActorID      string `json:"actor_id"`
}

type supervisionGraph struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

func newGraphNode(actor sup.Actor) graphNode {
	return graphNode{
		ID:   actor.ID(),
		Spec: inspectActor(actor),
	}
}

func (h *Hub) projectRuntimeEvent(event sup.Event) {
	if event.Type != sup.EventActorRegistered {
		return
	}

	actorNode := newGraphNode(event.Actor)
	var supervisorNode graphNode
	if event.Supervisor != nil {
		supervisorNode = newGraphNode(event.Supervisor)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	actorID := event.Actor.ID()
	h.graphNodes[actorID] = actorNode
	if event.Supervisor != nil {
		supervisorID := event.Supervisor.ID()
		h.graphNodes[supervisorID] = supervisorNode
		h.parents[actorID] = supervisorID
	}
}

func (h *Hub) graphSnapshot() supervisionGraph {
	h.mu.RLock()
	nodes := maps.Clone(h.graphNodes)
	parents := maps.Clone(h.parents)
	exposed := slices.Collect(maps.Keys(h.actors))
	h.mu.RUnlock()

	visible := make(map[string]struct{}, len(exposed))
	for _, actorID := range exposed {
		for id := actorID; id != ""; id = parents[id] {
			if _, exists := visible[id]; exists {
				break
			}
			visible[id] = struct{}{}
		}
	}

	graph := supervisionGraph{
		Nodes: make([]graphNode, 0, len(visible)),
		Edges: make([]graphEdge, 0, len(visible)),
	}

	for id := range visible {
		node, exists := nodes[id]
		if !exists {
			continue
		}
		graph.Nodes = append(graph.Nodes, node)

		parentID, hasParent := parents[id]
		if hasParent {
			if _, parentVisible := visible[parentID]; parentVisible {
				graph.Edges = append(graph.Edges, graphEdge{
					SupervisorID: parentID,
					ActorID:      id,
				})
			}
		}
	}

	slices.SortFunc(graph.Nodes, func(a, b graphNode) int {
		return cmp.Compare(a.ID, b.ID)
	})
	slices.SortFunc(graph.Edges, func(a, b graphEdge) int {
		if order := cmp.Compare(a.SupervisorID, b.SupervisorID); order != 0 {
			return order
		}
		return cmp.Compare(a.ActorID, b.ActorID)
	})

	return graph
}
