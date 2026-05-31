package hub

import (
	"reflect"
	"sort"

	"github.com/webermarci/sup"
)

type hubSignalValueType string

const (
	hubSignalValueTypeBoolean hubSignalValueType = "boolean"
	hubSignalValueTypeNumber  hubSignalValueType = "number"
	hubSignalValueTypeString  hubSignalValueType = "string"
	hubSignalValueTypeJSON    hubSignalValueType = "json"
)

type hubSignal struct {
	ID    string             `json:"id"`
	Spec  sup.Spec           `json:"spec"`
	Type  hubSignalValueType `json:"type"`
	Value any                `json:"value"`
}

func (h *Hub) signal(id string) (hubSignal, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	signal, ok := h.signals[id]
	return signal, ok
}

func (h *Hub) signalsSnapshot() []hubSignal {
	h.mu.RLock()
	defer h.mu.RUnlock()

	signals := make([]hubSignal, 0, len(h.signals))
	for _, signal := range h.signals {
		signals = append(signals, signal)
	}

	sort.Slice(signals, func(i, j int) bool {
		return signals[i].ID < signals[j].ID
	})

	return signals
}

func (h *Hub) updateSignalValue(id string, value any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	signal := h.signals[id]
	signal.Value = value
	h.signals[id] = signal
}

func inferSignalValueType[V any]() hubSignalValueType {
	t := reflect.TypeFor[V]()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		return hubSignalValueTypeBoolean
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return hubSignalValueTypeNumber
	case reflect.String:
		return hubSignalValueTypeString
	default:
		return hubSignalValueTypeJSON
	}
}
