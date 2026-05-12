package sup

import (
	"testing"
)

func TestBaseActor_Inspect(t *testing.T) {
	a := NewBaseActor("base")
	s := a.Inspect()

	if s.Kind != "actor" {
		t.Fatalf("expected kind actor, got %q", s.Kind)
	}

	if len(s.Dependencies) != 0 {
		t.Fatalf("expected no dependencies, got %v", s.Dependencies)
	}

	if s.Metadata == nil || len(s.Metadata) != 0 {
		t.Fatalf("expected empty metadata, got %v", s.Metadata)
	}
}
