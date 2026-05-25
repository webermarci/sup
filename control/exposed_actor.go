package control

import (
	"context"
	"encoding/json"

	"github.com/webermarci/sup"
)

type ActorOption func(*ExposedActor)

func Cast[T any](name string, fn func(context.Context, T) error) ActorOption {
	return func(ea *ExposedActor) {
		if ea.Control == nil {
			ea.Control = NewControl()
		}

		if _, exists := ea.Control.castsByName[name]; exists {
			panic("sup: duplicate cast: " + name)
		}
		if _, exists := ea.Control.callsByName[name]; exists {
			panic("sup: duplicate action: " + name)
		}

		action := &CastAction{
			Name:        name,
			InputSchema: schemaOf[T](),
			fn: func(ctx context.Context, raw json.RawMessage) error {
				var input T
				if err := json.Unmarshal(raw, &input); err != nil {
					return err
				}
				return fn(ctx, input)
			},
		}

		ea.Control.castsByName[name] = action
		ea.Control.Casts = append(ea.Control.Casts, action)
	}
}

func Call[T any, R any](name string, fn func(context.Context, T) (R, error)) ActorOption {
	return func(ea *ExposedActor) {
		if ea.Control == nil {
			ea.Control = NewControl()
		}

		if _, exists := ea.Control.callsByName[name]; exists {
			panic("sup: duplicate call: " + name)
		}
		if _, exists := ea.Control.castsByName[name]; exists {
			panic("sup: duplicate action: " + name)
		}

		action := &CallAction{
			Name:         name,
			InputSchema:  schemaOf[T](),
			OutputSchema: schemaOf[R](),
			fn: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input T
				if err := json.Unmarshal(raw, &input); err != nil {
					var zero R
					return zero, err
				}
				return fn(ctx, input)
			},
		}

		ea.Control.callsByName[name] = action
		ea.Control.Calls = append(ea.Control.Calls, action)
	}
}

type ExposedActor struct {
	ID      string   `json:"id"`
	Spec    sup.Spec `json:"spec"`
	Control *Control `json:"control,omitempty"`
}
