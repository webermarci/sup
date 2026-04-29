package sup

import (
	"sync"
)

var globalRegistry = NewRegistry()

// GlobalRegistry returns the default process registry.
func GlobalRegistry() *Registry {
	return globalRegistry
}

type Registry struct {
	actors map[string]Actor
	mu     sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		actors: make(map[string]Actor),
	}
}

// Register adds an actor to the registry. If an actor with the same name already exists, it will be overwritten.
func (r *Registry) Register(actor Actor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actors[actor.Name()] = actor
}

// Get retrieves an actor by name.
func (r *Registry) Get(name string) (Actor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actors[name]
	return a, ok
}

// Unregister removes an actor from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.actors, name)
}
