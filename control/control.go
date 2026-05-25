package control

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
)

var (
	ErrCastNotFound = errors.New("sup: cast not found")
	ErrCallNotFound = errors.New("sup: call not found")
)

type CastAction struct {
	Name        string         `json:"name"`
	InputSchema map[string]any `json:"input_schema"`

	fn func(context.Context, json.RawMessage) error
}

type CallAction struct {
	Name         string         `json:"name"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema"`

	fn func(context.Context, json.RawMessage) (any, error)
}

type Control struct {
	Casts []*CastAction `json:"casts"`
	Calls []*CallAction `json:"calls"`

	castsByName map[string]*CastAction
	callsByName map[string]*CallAction
}

func NewControl() *Control {
	return &Control{
		castsByName: make(map[string]*CastAction),
		callsByName: make(map[string]*CallAction),
	}
}

func (c *Control) Cast(ctx context.Context, name string, raw json.RawMessage) error {
	action, ok := c.castsByName[name]
	if !ok {
		return ErrCastNotFound
	}

	return action.fn(ctx, raw)
}

func (c *Control) Call(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	action, ok := c.callsByName[name]
	if !ok {
		return nil, ErrCallNotFound
	}

	return action.fn(ctx, raw)
}

func typeOf[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}

func schemaOf[T any]() map[string]any {
	return schemaOfType(typeOf[T]())
}

func schemaOfType(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{"type": "null"}
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaOfType(t.Elem()),
		}

	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": schemaOfType(t.Elem()),
			}
		}
		return map[string]any{"type": "object"}

	case reflect.Struct:
		return schemaOfStruct(t)

	case reflect.Interface:
		return map[string]any{}

	default:
		return map[string]any{"type": "object"}
	}
}

func schemaOfStruct(t reflect.Type) map[string]any {
	properties := map[string]any{}
	required := []string{}

	for field := range t.Fields() {
		if field.PkgPath != "" {
			continue
		}

		name, omitempty, skip := jsonFieldName(field)
		if skip {
			continue
		}

		properties[name] = schemaOfType(field.Type)

		if !omitempty {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func jsonFieldName(field reflect.StructField) (name string, omitempty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}

	if tag == "" {
		return field.Name, false, false
	}

	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, true
	}

	name = parts[0]
	if name == "" {
		name = field.Name
	}

	if slices.Contains(parts[1:], "omitempty") {
		omitempty = true
	}

	return name, omitempty, false
}
