package ui

import "reflect"

// Card represents a single card in the dashboard, with a name and a type.
type Card struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func inferType[V any]() string {
	t := reflect.TypeFor[V]()
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.String:
		return "string"
	default:
		return "json"
	}
}
