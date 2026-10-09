package schema

import "testing"

func TestValidate(t *testing.T) {
	valid := map[string]any{
		"schema_version": "v1.1",
		"pack_namespace": "x",
		"profiles":       map[string]any{},
		"providers":      map[string]any{},
		"namespaces":     map[string]any{},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	bad := map[string]any{
		"schema_version": "v9.9",
		"pack_namespace": "x",
		"profiles":       map[string]any{},
		"providers":      map[string]any{},
		"namespaces":     map[string]any{},
	}
	if err := Validate(bad); err == nil {
		t.Fatal("expected invalid schema_version to be rejected")
	}
}
