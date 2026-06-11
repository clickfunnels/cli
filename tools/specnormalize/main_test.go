package main

import "testing"

func norm(m map[string]any) map[string]any {
	return normalize(m).(map[string]any)
}

// type: [X, "null"] collapses to a single scalar, and `nullable` is stripped
// (so codegen emits *T,omitempty).
func TestNormalizeNullableTypeArray(t *testing.T) {
	got := norm(map[string]any{"type": []any{"string", "null"}})
	if got["type"] != "string" {
		t.Errorf("type = %v, want string", got["type"])
	}
	if _, ok := got["nullable"]; ok {
		t.Errorf("nullable should be stripped, got %v", got["nullable"])
	}
}

// A malformed enum (not a list — e.g. the leaked Ruby expression) is dropped;
// a valid enum is kept.
func TestNormalizeEnums(t *testing.T) {
	bad := norm(map[string]any{"type": "string", "enum": "I18n.available_locales.map(&:to_s)"})
	if _, ok := bad["enum"]; ok {
		t.Errorf("malformed enum should be dropped, got %v", bad["enum"])
	}
	good := norm(map[string]any{"type": "string", "enum": []any{"en", "ja"}})
	if _, ok := good["enum"].([]any); !ok {
		t.Errorf("valid enum should be kept, got %v", good["enum"])
	}
}

// An integer|string id union (all-scalar oneOf) collapses to the string type.
func TestNormalizeScalarUnion(t *testing.T) {
	got := norm(map[string]any{"oneOf": []any{
		map[string]any{"type": "integer"},
		map[string]any{"type": "string"},
	}})
	if got["type"] != "string" {
		t.Errorf("type = %v, want string", got["type"])
	}
	if _, ok := got["oneOf"]; ok {
		t.Errorf("oneOf should be collapsed away, got %v", got["oneOf"])
	}
}

// A nullable $ref idiom (oneOf of a $ref and {type: null}) flattens to the ref,
// with nullable stripped.
func TestNormalizeNullableRef(t *testing.T) {
	got := norm(map[string]any{"oneOf": []any{
		map[string]any{"$ref": "#/components/schemas/Foo"},
		map[string]any{"type": "null"},
	}})
	if got["$ref"] != "#/components/schemas/Foo" {
		t.Errorf("$ref = %v, want the schema ref", got["$ref"])
	}
	if _, ok := got["oneOf"]; ok {
		t.Errorf("oneOf should be flattened away, got %v", got["oneOf"])
	}
	if _, ok := got["nullable"]; ok {
		t.Errorf("nullable should be stripped, got %v", got["nullable"])
	}
}

// Normalization recurses into nested schemas (e.g. object properties).
func TestNormalizeRecurses(t *testing.T) {
	got := norm(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email": map[string]any{"type": []any{"string", "null"}},
		},
	})
	props := got["properties"].(map[string]any)
	email := props["email"].(map[string]any)
	if email["type"] != "string" {
		t.Errorf("nested type = %v, want string", email["type"])
	}
	if _, ok := email["nullable"]; ok {
		t.Errorf("nested nullable should be stripped")
	}
}
