package sup

import (
	"reflect"
	"slices"
	"strings"
)

// ControlKind identifies how a control dispatches input to an actor.
type ControlKind string

const (
	// ControlKindCall identifies a control that returns a reply.
	ControlKindCall ControlKind = "call"

	// ControlKindCast identifies a control that does not return a reply.
	ControlKindCast ControlKind = "cast"
)

// Control describes an actor command that can be dispatched dynamically.
type Control interface {
	Name() string
	Kind() ControlKind
	InputSchema() map[string]any
}

// Controllable represents an actor that exposes dispatchable controls.
type Controllable interface {
	Controls() []Control
}

func schemaOf[T any]() map[string]any {
	return schemaOfType(typeOf[T]())
}

func typeOf[T any]() reflect.Type {
	return reflect.TypeFor[T]()
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
