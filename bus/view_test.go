package bus

import (
	"testing"
)

type mockReadable[V any] struct {
	value V
}

func (m *mockReadable[V]) Read() V {
	return m.value
}

func TestViewFunc_Simple(t *testing.T) {
	parent := &mockReadable[int]{value: 10}

	// Just cast the function to ViewFunc[int]
	view := ViewFunc[int](func() int {
		return parent.Read() * 2
	})

	if got := view.Read(); got != 20 {
		t.Errorf("Expected 20, got %d", got)
	}

	// Because it's lazy, changing the parent instantly changes the view
	parent.value = 50
	if got := view.Read(); got != 100 {
		t.Errorf("Expected 100, got %d", got)
	}
}

func TestViewFunc_Aggregation(t *testing.T) {
	p1 := &mockReadable[int]{value: 10}
	p2 := &mockReadable[int]{value: 20}

	sum := ViewFunc[int](func() int {
		return p1.Read() + p2.Read()
	})

	if got := sum.Read(); got != 30 {
		t.Errorf("Expected 30, got %d", got)
	}
}

func TestViewFunc_TypeConversion(t *testing.T) {
	parent := &mockReadable[int]{value: 1}

	view := ViewFunc[string](func() string {
		if parent.Read() == 1 {
			return "Active"
		}
		return "Inactive"
	})

	if got := view.Read(); got != "Active" {
		t.Errorf("Expected 'Active', got %s", got)
	}

	parent.value = 0
	if got := view.Read(); got != "Inactive" {
		t.Errorf("Expected 'Inactive', got %s", got)
	}
}

func TestViewFunc_Chaining(t *testing.T) {
	p := &mockReadable[int]{value: 10}

	// ViewFunc 1 depends on mock
	v1 := ViewFunc[int](func() int {
		return p.Read() * 2
	})

	// ViewFunc 2 depends on ViewFunc 1
	v2 := ViewFunc[int](func() int {
		return v1.Read() + 5
	})

	if got := v2.Read(); got != 25 {
		t.Errorf("Expected 25, got %d", got)
	}

	// Pushing a change up the chain
	p.value = 20
	// 20 * 2 = 40, 40 + 5 = 45
	if got := v2.Read(); got != 45 {
		t.Errorf("Expected 45, got %d", got)
	}
}
