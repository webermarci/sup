package ui

import "reflect"

// CardMode represents the mode of a card, which can be either "read" for displaying values or "write" for allowing user input to update values.
type CardMode string

const (
	CardModeRead  CardMode = "read"
	CardModeWrite CardMode = "write"
)

// Card represents a UI component on the dashboard that can either display a value (read mode) or allow user input to update a value (write mode). Each card has a name that identifies the data it represents and a mode that determines whether it is read-only or interactive.
type Card struct {
	Name string   `json:"name"`
	Mode CardMode `json:"mode"`
	Type string   `json:"type"`
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
