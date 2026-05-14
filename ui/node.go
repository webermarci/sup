package ui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

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

func formatValue(v any) string {
	if v == nil {
		return ""
	}

	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.RawMessage:
		return string(x)
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 64)
	case reflect.Map, reflect.Struct, reflect.Slice, reflect.Array:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}
