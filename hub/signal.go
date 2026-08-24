package hub

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"maps"
	"reflect"
	"slices"

	"github.com/webermarci/sup/rx"
)

var errSignalReadOnly = errors.New("hub: signal is read-only")

type hubSignalValueType string

const (
	hubSignalValueTypeBoolean hubSignalValueType = "boolean"
	hubSignalValueTypeNumber  hubSignalValueType = "number"
	hubSignalValueTypeString  hubSignalValueType = "string"
	hubSignalValueTypeJSON    hubSignalValueType = "json"
)

type hubSignal struct {
	ID       string             `json:"id"`
	Type     hubSignalValueType `json:"type"`
	Value    any                `json:"value"`
	Writable bool               `json:"writable"`
}

type registeredSignal struct {
	id        string
	valueType hubSignalValueType
	writable  bool
	read      func() any
	write     func(jsontext.Value) error
	observe   func(context.Context, func(any))
}

func newReadableSignal[V any](signal rx.Readable[V]) registeredSignal {
	return registeredSignal{
		id:        signal.ID(),
		valueType: inferSignalValueType[V](),
		read:      func() any { return signal.Value() },
		observe: func(ctx context.Context, publish func(any)) {
			observeSignal(ctx, signal, publish)
		},
	}
}

func newWritableSignal[V any](signal rx.Writable[V]) registeredSignal {
	registered := newReadableSignal(signal)
	registered.writable = true
	registered.write = func(raw jsontext.Value) error {
		var value V
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		signal.Set(value)
		return nil
	}
	return registered
}

func observeSignal[V any](ctx context.Context, signal rx.Readable[V], publish func(any)) {
	values := signal.Subscribe(ctx)

	// Subscribe starts with the current value. It is already available from the
	// snapshot endpoints and is not a change event.
	select {
	case _, ok := <-values:
		if !ok {
			return
		}
	case <-ctx.Done():
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case value, ok := <-values:
			if !ok {
				return
			}
			publish(value)
		}
	}
}

func (s registeredSignal) describe() hubSignal {
	return hubSignal{
		ID:       s.id,
		Type:     s.valueType,
		Value:    s.read(),
		Writable: s.writable,
	}
}

func (s registeredSignal) set(raw jsontext.Value) error {
	if s.write == nil {
		return errSignalReadOnly
	}
	return s.write(raw)
}

func (h *Hub) signalsSnapshot() []hubSignal {
	signals := slices.SortedFunc(maps.Values(h.signals), func(a, b registeredSignal) int {
		return cmp.Compare(a.id, b.id)
	})

	result := make([]hubSignal, 0, len(signals))
	for _, signal := range signals {
		result = append(result, signal.describe())
	}
	return result
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
