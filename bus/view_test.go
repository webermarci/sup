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

func TestView_Simple(t *testing.T) {
	parent := &mockReadable[int]{value: 10}

	m := NewView(t.Name(), func() int {
		return parent.Read() * 2
	})

	if got := m.Read(); got != 20 {
		t.Errorf("Expected 20, got %d", got)
	}

	parent.value = 50
	if got := m.Read(); got != 100 {
		t.Errorf("Expected 100, got %d", got)
	}
}

func TestView_Aggregation(t *testing.T) {
	p1 := &mockReadable[int]{value: 10}
	p2 := &mockReadable[int]{value: 20}

	sum := NewView(t.Name(), func() int {
		return p1.Read() + p2.Read()
	})

	if got := sum.Read(); got != 30 {
		t.Errorf("Expected 30, got %d", got)
	}
}

func TestView_TypeConversion(t *testing.T) {
	parent := &mockReadable[int]{value: 1}

	m := NewView(t.Name(), func() string {
		if parent.Read() == 1 {
			return "Active"
		}
		return "Inactive"
	})

	if got := m.Read(); got != "Active" {
		t.Errorf("Expected 'Active', got %s", got)
	}
}

func TestView_Chaining(t *testing.T) {
	p := &mockReadable[int]{value: 10}

	m1 := NewView(t.Name(), func() int {
		return p.Read() * 2
	})

	m2 := NewView(t.Name(), func() int {
		return m1.Read() + 5
	})

	if got := m2.Read(); got != 25 {
		t.Errorf("Expected 25, got %d", got)
	}
}
