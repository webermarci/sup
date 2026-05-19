package ui

import (
	"reflect"

	"github.com/webermarci/sup"
)

type Node struct {
	Name  string   `json:"name"`
	Spec  sup.Spec `json:"spec"`
	Type  string   `json:"type"`
	Value any      `json:"value"`
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
