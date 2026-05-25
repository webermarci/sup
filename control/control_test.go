package control

import (
	"slices"
	"testing"
)

type schemaNested struct {
	Enabled bool `json:"enabled"`
}

type schemaInput struct {
	Name     string            `json:"name"`
	Age      int               `json:"age,omitempty"`
	Scores   []float64         `json:"scores"`
	Labels   map[string]string `json:"labels"`
	Nested   *schemaNested     `json:"nested,omitempty"`
	Ignored  string            `json:"-"`
	unexport string
}

func TestSchemaOfStruct(t *testing.T) {
	schema := schemaOf[schemaInput]()

	if schema["type"] != "object" {
		t.Fatalf("expected object schema, got %#v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %#v", schema["properties"])
	}

	if _, ok := props["name"]; !ok {
		t.Fatal("expected name property")
	}

	if _, ok := props["age"]; !ok {
		t.Fatal("expected age property")
	}

	if _, ok := props["ignored"]; ok {
		t.Fatal("did not expect ignored property")
	}

	if _, ok := props["unexport"]; ok {
		t.Fatal("did not expect unexported property")
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("expected required []string, got %#v", schema["required"])
	}

	if !slices.Contains(required, "name") {
		t.Fatal("expected name to be required")
	}

	if slices.Contains(required, "age") {
		t.Fatal("did not expect age to be required")
	}
}

func TestSchemaOfPrimitiveTypes(t *testing.T) {
	tests := []struct {
		name string
		got  map[string]any
		want string
	}{
		{"string", schemaOf[string](), "string"},
		{"bool", schemaOf[bool](), "boolean"},
		{"int", schemaOf[int](), "integer"},
		{"float64", schemaOf[float64](), "number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got["type"] != tt.want {
				t.Fatalf("expected %q, got %#v", tt.want, tt.got["type"])
			}
		})
	}
}

func TestSchemaOfSlice(t *testing.T) {
	schema := schemaOf[[]int]()

	if schema["type"] != "array" {
		t.Fatalf("expected array, got %#v", schema["type"])
	}

	items, ok := schema["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected items schema, got %#v", schema["items"])
	}

	if items["type"] != "integer" {
		t.Fatalf("expected integer items, got %#v", items["type"])
	}
}

func TestSchemaOfMap(t *testing.T) {
	schema := schemaOf[map[string]int]()

	if schema["type"] != "object" {
		t.Fatalf("expected object, got %#v", schema["type"])
	}

	additional, ok := schema["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("expected additionalProperties schema, got %#v", schema["additionalProperties"])
	}

	if additional["type"] != "integer" {
		t.Fatalf("expected integer additionalProperties, got %#v", additional["type"])
	}
}

func TestSchemaOfPointer(t *testing.T) {
	schema := schemaOf[*schemaNested]()

	if schema["type"] != "object" {
		t.Fatalf("expected object, got %#v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %#v", schema["properties"])
	}

	if _, ok := props["enabled"]; !ok {
		t.Fatal("expected enabled property")
	}
}
