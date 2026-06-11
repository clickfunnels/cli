package main

import "testing"

func TestExtractOps(t *testing.T) {
	root := map[string]any{
		"paths": map[string]any{
			"/teams": map[string]any{
				"get": map[string]any{"operationId": "listTeams", "summary": "List teams", "tags": []any{"Team"}},
			},
			"/contacts/{id}": map[string]any{
				"get":    map[string]any{"operationId": "getContact", "tags": []any{"Contact"}},
				"delete": map[string]any{"operationId": "removeContact", "tags": []any{"Contact"}},
			},
		},
	}

	ops := extractOps(root)

	// Sorted by path ("/contacts/{id}" < "/teams"), then method (get < delete).
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d: %+v", len(ops), ops)
	}
	want := []struct{ opID, method, path, tag string }{
		{"getContact", "get", "/contacts/{id}", "Contact"},
		{"removeContact", "delete", "/contacts/{id}", "Contact"},
		{"listTeams", "get", "/teams", "Team"},
	}
	for i, w := range want {
		got := ops[i]
		if got.OpID != w.opID || got.Method != w.method || got.Path != w.path || got.Tag != w.tag {
			t.Errorf("ops[%d] = %+v, want %v", i, got, w)
		}
	}
}

func TestFirstTag(t *testing.T) {
	if got := firstTag(map[string]any{"tags": []any{"Foo", "Bar"}}); got != "Foo" {
		t.Errorf("firstTag = %q, want Foo", got)
	}
	if got := firstTag(map[string]any{}); got != "Other" {
		t.Errorf("firstTag (no tags) = %q, want Other", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("one\ntwo\nthree"); got != "one" {
		t.Errorf("firstLine = %q, want one", got)
	}
	if got := firstLine("solo"); got != "solo" {
		t.Errorf("firstLine = %q, want solo", got)
	}
}
