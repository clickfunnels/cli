// Package output renders command results in a chosen format: a styled ANSI
// table (default), or JSON / YAML / CSV for piping and scripting.
//
// Three entry points:
//   - Collection: a typed slice + column definitions (used by curated commands
//     that know their shape). JSON/YAML emit the full objects; table/CSV use the
//     columns.
//   - Object: a single typed value.
//   - Raw: an undecoded JSON body from the API (used by the generated command
//     layer, which doesn't have typed shapes on hand).
package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/clickfunnels/cli/internal/ui"
)

// Format is an output format.
type Format string

const (
	Table Format = "table"
	JSON  Format = "json"
	YAML  Format = "yaml"
	CSV   Format = "csv"
)

// Parse validates and returns a Format.
func Parse(s string) (Format, error) {
	switch Format(s) {
	case Table, JSON, YAML, CSV:
		return Format(s), nil
	default:
		return "", fmt.Errorf("invalid --output %q (want: table, json, yaml, csv)", s)
	}
}

// Column maps a header to a cell extractor for a row of type T.
type Column[T any] struct {
	Header string
	Value  func(T) string
}

// Collection renders items in the chosen format. JSON/YAML marshal the full
// objects; table/CSV use the columns.
func Collection[T any](w io.Writer, f Format, cols []Column[T], items []T) error {
	switch f {
	case JSON:
		return writeJSON(w, items)
	case YAML:
		return writeYAML(w, items)
	case CSV:
		return writeCSV(w, headers(cols), rowsOf(cols, items))
	default:
		return writeTable(w, headers(cols), rowsOf(cols, items))
	}
}

// Object renders a single value. JSON/YAML marshal it; table/CSV render a
// key/value (field/value) view from its JSON representation.
func Object(w io.Writer, f Format, v any) error {
	switch f {
	case JSON:
		return writeJSON(w, v)
	case YAML:
		return writeYAML(w, v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return Raw(w, f, b)
	}
}

// Raw renders an undecoded JSON body. For JSON it pretty-prints; for YAML it
// re-encodes; for table/CSV it builds columns from a top-level array of objects
// (or a key/value view for a single object).
func Raw(w io.Writer, f Format, body []byte) error {
	if f == JSON {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			body = pretty.Bytes()
		}
		_, err := fmt.Fprintln(w, strings.TrimRight(string(body), "\n"))
		return err
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// Not JSON (e.g. an empty body) — pass it through verbatim.
		_, err := fmt.Fprintln(w, strings.TrimRight(string(body), "\n"))
		return err
	}

	if f == YAML {
		return writeYAML(w, decoded)
	}

	// table / csv: derive columns from the data shape.
	hdr, rows := tabulate(decoded)
	if f == CSV {
		return writeCSV(w, hdr, rows)
	}
	return writeTable(w, hdr, rows)
}

// --- formatters ---

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeYAML(w io.Writer, v any) error {
	// Round-trip through JSON first so json tags (snake_case) drive the keys,
	// rather than Go field names.
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return err
	}
	out, err := yaml.Marshal(generic)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

func writeCSV(w io.Writer, headers []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if len(headers) > 0 {
		if err := cw.Write(headers); err != nil {
			return err
		}
	}
	if err := cw.WriteAll(rows); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	if len(rows) == 0 && len(headers) == 0 {
		_, err := fmt.Fprintln(w, ui.Subtle.Render("No results."))
		return err
	}
	_, err := fmt.Fprintln(w, ui.RenderTable(headers, rows))
	return err
}

func headers[T any](cols []Column[T]) []string {
	h := make([]string, len(cols))
	for i, c := range cols {
		h[i] = c.Header
	}
	return h
}

func rowsOf[T any](cols []Column[T], items []T) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = c.Value(it)
		}
		rows = append(rows, row)
	}
	return rows
}

// tabulate derives headers + rows from decoded JSON: an array of objects becomes
// a column per (unioned, sorted) key; a single object becomes FIELD/VALUE rows.
func tabulate(decoded any) (headers []string, rows [][]string) {
	switch v := decoded.(type) {
	case []any:
		keySet := map[string]bool{}
		for _, el := range v {
			if m, ok := el.(map[string]any); ok {
				for k := range m {
					keySet[k] = true
				}
			}
		}
		headers = sortedKeys(keySet)
		for _, el := range v {
			m, _ := el.(map[string]any)
			row := make([]string, len(headers))
			for i, k := range headers {
				row[i] = cell(m[k])
			}
			rows = append(rows, row)
		}
	case map[string]any:
		headers = []string{"FIELD", "VALUE"}
		for _, k := range sortedKeys(keysOf(v)) {
			rows = append(rows, []string{k, cell(v[k])})
		}
	default:
		headers = []string{"VALUE"}
		rows = [][]string{{cell(decoded)}}
	}
	return headers, rows
}

// cell renders a JSON value as a single table/CSV cell. Scalars print plainly;
// objects and arrays are compact JSON.
func cell(v any) string {
	switch v.(type) {
	case nil:
		return ""
	case string, float64, bool, json.Number:
		return fmt.Sprintf("%v", v)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func keysOf(m map[string]any) map[string]bool {
	set := make(map[string]bool, len(m))
	for k := range m {
		set[k] = true
	}
	return set
}
