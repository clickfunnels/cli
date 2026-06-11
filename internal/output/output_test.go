package output

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
	"testing"
)

type row struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func cols() []Column[row] {
	return []Column[row]{
		{Header: "NAME", Value: func(r row) string { return r.Name }},
		{Header: "COUNT", Value: func(r row) string { return strconv.Itoa(r.Count) }},
	}
}

func TestParse(t *testing.T) {
	for _, ok := range []string{"table", "json", "yaml", "csv"} {
		if _, err := Parse(ok); err != nil {
			t.Errorf("Parse(%q) errored: %v", ok, err)
		}
	}
	if _, err := Parse("toml"); err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestCollectionJSON(t *testing.T) {
	var b bytes.Buffer
	if err := Collection(&b, JSON, cols(), []row{{Name: "a", Count: 1}}); err != nil {
		t.Fatal(err)
	}
	// JSON emits full objects with json-tag keys, not just the columns.
	if !strings.Contains(b.String(), `"name": "a"`) || !strings.Contains(b.String(), `"count": 1`) {
		t.Errorf("unexpected JSON: %s", b.String())
	}
}

func TestCollectionCSV(t *testing.T) {
	var b bytes.Buffer
	if err := Collection(&b, CSV, cols(), []row{{Name: "a", Count: 1}, {Name: "b", Count: 2}}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(b.String())
	want := "NAME,COUNT\na,1\nb,2"
	if got != want {
		t.Errorf("CSV =\n%q\nwant\n%q", got, want)
	}
}

func TestCollectionYAML(t *testing.T) {
	var b bytes.Buffer
	if err := Collection(&b, YAML, cols(), []row{{Name: "a", Count: 1}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "name: a") || !strings.Contains(b.String(), "count: 1") {
		t.Errorf("unexpected YAML: %s", b.String())
	}
}

func TestRawCSVFromArray(t *testing.T) {
	var b bytes.Buffer
	body := []byte(`[{"id":1,"name":"x"},{"id":2,"name":"y"}]`)
	if err := Raw(&b, CSV, body); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(b.String())
	// keys are unioned + sorted: id,name
	want := "id,name\n1,x\n2,y"
	if got != want {
		t.Errorf("Raw CSV =\n%q\nwant\n%q", got, want)
	}
}

func TestRawYAMLFromObject(t *testing.T) {
	var b bytes.Buffer
	if err := Raw(&b, YAML, []byte(`{"a":1,"b":"two"}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "a: 1") || !strings.Contains(b.String(), "b: two") {
		t.Errorf("unexpected YAML: %s", b.String())
	}
}

func TestRawCSVNestedValuesAreJSON(t *testing.T) {
	var b bytes.Buffer
	body := []byte(`[{"id":1,"tags":["a","b"]}]`)
	if err := Raw(&b, CSV, body); err != nil {
		t.Fatal(err)
	}
	// Read the CSV back so quoting is handled; the tags cell should be compact JSON.
	records, err := csv.NewReader(strings.NewReader(b.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// header: id,tags ; row: 1,["a","b"]
	if len(records) != 2 || records[0][1] != "tags" || records[1][1] != `["a","b"]` {
		t.Errorf("unexpected CSV records: %#v", records)
	}
}
