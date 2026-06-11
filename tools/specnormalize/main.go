// Command specnormalize rewrites an OpenAPI spec so it is "codegen-clean" for
// oapi-codegen, which does not yet fully support OpenAPI 3.1 / JSON Schema
// niceties. It is the middle step of the generate pipeline:
//
//	openapi.yaml (3.1)
//	  -> openapi-down-convert (3.1 -> 3.0)   [npx @apiture/openapi-down-convert]
//	  -> specnormalize (this tool)           [collapse residual unions]
//	  -> oapi-codegen                        [Go models]
//
// What it normalizes:
//
//   - `type: [X, "null"]` arrays -> `type: X` plus `nullable: true`
//   - `type: ["null"]`           -> dropped (left untyped)
//   - mixed primitive unions like `type: [integer, string]` -> first non-null
//     member (the ClickFunnels obfuscated-id-or-numeric-id pattern; clients use
//     the string public id)
//   - `oneOf`/`anyOf` branches that are just `{type: "null"}` -> removed, and a
//     single-remaining-branch oneOf/anyOf is flattened to that branch with
//     `nullable: true`
//
// Usage: specnormalize <input.yaml> <output.yaml>
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: specnormalize <input.yaml> <output.yaml>")
		os.Exit(2)
	}
	in, err := os.ReadFile(os.Args[1])
	must(err)

	var root any
	must(yaml.Unmarshal(in, &root))

	root = normalize(root)

	out, err := yaml.Marshal(root)
	must(err)
	must(os.WriteFile(os.Args[2], out, 0o644))
}

// normalize walks the decoded YAML tree and rewrites schema nodes in place.
func normalize(node any) any {
	switch n := node.(type) {
	case map[string]any:
		// Drop malformed enums. The published spec ships a literal,
		// un-evaluated Ruby expression as an enum value
		// (`enum: I18n.available_locales.map(&:to_s)`), which decodes to a
		// string rather than a list. An enum that isn't a list is invalid;
		// drop it so the field degrades to its base type.
		if raw, ok := n["enum"]; ok {
			if _, isList := raw.([]any); !isList {
				delete(n, "enum")
			}
		}

		// Collapse a `type` sequence into a single scalar (+ nullable).
		if raw, ok := n["type"]; ok {
			if seq, ok := raw.([]any); ok {
				nonNull := make([]any, 0, len(seq))
				hadNull := false
				for _, t := range seq {
					if s, _ := t.(string); s == "null" {
						hadNull = true
						continue
					}
					nonNull = append(nonNull, t)
				}
				if len(nonNull) >= 1 {
					n["type"] = nonNull[0] // pick the first concrete type
				} else {
					delete(n, "type")
				}
				if hadNull {
					n["nullable"] = true
				}
			}
		}

		// Clean oneOf/anyOf null branches.
		for _, key := range []string{"oneOf", "anyOf"} {
			if raw, ok := n[key]; ok {
				if seq, ok := raw.([]any); ok {
					cleanUnion(n, key, seq)
				}
			}
		}

		for k, v := range n {
			n[k] = normalize(v)
		}

		// Drop `nullable`. For a typed *client* we never need to send an
		// explicit JSON null (we omit instead), and on reads a null decodes to a
		// nil pointer either way. Removing it makes oapi-codegen emit optional
		// fields as `*T` with `,omitempty`, so partial updates (PATCH/PUT) send
		// only the fields the caller set rather than nulling everything else.
		delete(n, "nullable")
		return n

	case map[any]any: // yaml.v3 can yield this for some maps
		m := make(map[string]any, len(n))
		for k, v := range n {
			m[fmt.Sprintf("%v", k)] = v
		}
		return normalize(m)

	case []any:
		for i, v := range n {
			n[i] = normalize(v)
		}
		return n
	}
	return node
}

// cleanUnion removes `{type: null}` branches and mutates parent in place. If
// exactly one branch remains and it is a $ref, it flattens the union onto the
// parent as a nullable ref.
func cleanUnion(parent map[string]any, key string, seq []any) {
	kept := make([]any, 0, len(seq))
	dropped := false
	for _, b := range seq {
		if bm, ok := b.(map[string]any); ok {
			if t, _ := bm["type"].(string); t == "null" && len(bm) == 1 {
				dropped = true
				continue
			}
		}
		kept = append(kept, b)
	}
	if dropped {
		parent["nullable"] = true
	}

	// If every remaining branch is a primitive scalar schema (e.g. the
	// integer|string obfuscated-id pattern), collapse to a single scalar type.
	// Go can't represent a true union here, and clients use the string form.
	if len(kept) >= 1 && allScalar(kept) {
		delete(parent, key)
		parent["type"] = preferredScalar(kept)
		return
	}

	if len(kept) == 1 {
		if bm, ok := kept[0].(map[string]any); ok {
			if ref, ok := bm["$ref"]; ok {
				// Flatten: parent becomes the ref (nullable already set above).
				delete(parent, key)
				parent["$ref"] = ref
				return
			}
		}
	}
	parent[key] = kept
}

// allScalar reports whether every branch is a primitive-typed schema with no
// $ref, properties, items, or nested composition.
func allScalar(branches []any) bool {
	for _, b := range branches {
		bm, ok := b.(map[string]any)
		if !ok {
			return false
		}
		t, ok := bm["type"].(string)
		if !ok {
			return false
		}
		switch t {
		case "string", "integer", "number", "boolean":
			// ok
		default:
			return false
		}
		for _, bad := range []string{"$ref", "properties", "items", "oneOf", "anyOf", "allOf"} {
			if _, present := bm[bad]; present {
				return false
			}
		}
	}
	return true
}

// preferredScalar returns "string" if any branch is a string, else the first
// branch's type. The string form matches ClickFunnels' obfuscated public ids.
func preferredScalar(branches []any) string {
	first := ""
	for _, b := range branches {
		t, _ := b.(map[string]any)["type"].(string)
		if first == "" {
			first = t
		}
		if t == "string" {
			return "string"
		}
	}
	return first
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "specnormalize:", err)
		os.Exit(1)
	}
}
