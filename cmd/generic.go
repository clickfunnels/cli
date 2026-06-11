package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/clickfunnels/cli/internal/api"
	"github.com/clickfunnels/cli/internal/output"
)

// opSpec is one API operation from the generated manifest (operations.gen.go).
type opSpec struct {
	Tag     string
	OpID    string
	Summary string
	Method  string
	Path    string
}

// curatedTags have hand-built, polished commands (tables, forms), so we skip
// generating generic equivalents for them to avoid two ways of doing the same
// thing. Everything else in the API is exposed generically.
var curatedTags = map[string]bool{
	"Team":        true,
	"Contact":     true,
	"Blogs::Post": true,
}

const genericGroupID = "api"

// mountGenerated builds the generic, spec-driven command tree and attaches it to
// root: one group per API tag, one leaf per operation. Leaves issue the request
// through the same authed transport and print JSON — full coverage of the API
// without a hand-written command per endpoint.
func mountGenerated(root *cobra.Command) {
	root.AddGroup(&cobra.Group{ID: genericGroupID, Title: "Other API resources (generated from the spec; JSON output):"})

	groups := map[string]*cobra.Command{}
	usedLeaf := map[string]map[string]bool{}

	for _, o := range generatedOperations {
		if curatedTags[o.Tag] || o.OpID == "" {
			continue
		}
		gname := kebab(o.Tag)
		g := groups[gname]
		if g == nil {
			g = &cobra.Command{
				Use:     gname,
				Short:   "Manage " + o.Tag,
				GroupID: genericGroupID,
			}
			groups[gname] = g
			usedLeaf[gname] = map[string]bool{}
		}
		leaf := leadingVerb(o.OpID)
		if usedLeaf[gname][leaf] {
			leaf = kebab(o.OpID) // disambiguate a verb collision within a tag
		}
		usedLeaf[gname][leaf] = true
		g.AddCommand(newGenericCmd(o, leaf))
	}

	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		root.AddCommand(groups[n])
	}
}

func newGenericCmd(o opSpec, leaf string) *cobra.Command {
	c := &cobra.Command{
		Use:   leaf,
		Short: o.Summary,
		Long:  fmt.Sprintf("%s\n\n%s %s", o.Summary, o.Method, o.Path),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGeneric(cmd, o)
		},
	}
	for _, p := range pathParams(o.Path) {
		desc := "path parameter (required)"
		if p == "workspace_id" {
			desc = "path parameter (defaults to the active workspace)"
		}
		c.Flags().String(kebab(p), "", desc)
	}
	c.Flags().StringArray("query", nil, "query parameter key=value (repeatable)")
	c.Flags().StringArrayP("field", "f", nil, "JSON body field key=value (repeatable)")
	c.Flags().String("input", "", "request body from a file, or '-' for stdin")
	return c
}

func runGeneric(cmd *cobra.Command, o opSpec) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	_, account, err := authedClient(cmd)
	if err != nil {
		return err
	}

	// Substitute path parameters.
	path := o.Path
	for _, name := range pathParams(o.Path) {
		v, _ := cmd.Flags().GetString(kebab(name))
		if v == "" && name == "workspace_id" && account.WorkspaceID != 0 {
			v = strconv.FormatInt(account.WorkspaceID, 10)
		}
		if v == "" {
			return fmt.Errorf("missing required path parameter --%s", kebab(name))
		}
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(v))
	}

	// Query parameters.
	queries, _ := cmd.Flags().GetStringArray("query")
	q := url.Values{}
	for _, kv := range queries {
		k, val, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("--query must be key=value, got %q", kv)
		}
		q.Add(k, val)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	// Request body: --input (raw JSON, incl. any wrapper) or -f key=value pairs.
	input, _ := cmd.Flags().GetString("input")
	fields, _ := cmd.Flags().GetStringArray("field")
	var body []byte
	switch {
	case input != "" && len(fields) > 0:
		return fmt.Errorf("pass only one of --input or --field")
	case input != "":
		if body, err = readInput(input); err != nil {
			return err
		}
	case len(fields) > 0:
		m := map[string]string{}
		for _, kv := range fields {
			k, val, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("--field must be key=value, got %q", kv)
			}
			m[k] = val
		}
		if body, err = json.Marshal(m); err != nil {
			return err
		}
	}

	resp, err := api.RawRequest(cmd.Context(), account.BaseURL(), account.AccessToken, o.Method, path, body)
	if err != nil {
		return err
	}

	if err := output.Raw(os.Stdout, format, resp.Body); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

var pathParamRE = regexp.MustCompile(`\{([^}]+)\}`)

// pathParams returns the {placeholder} names in a path template, in order.
func pathParams(path string) []string {
	matches := pathParamRE.FindAllStringSubmatch(path, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// leadingVerb returns the action prefix of an operationId, e.g. "list" from
// "listOrdersInvoices" — the run of characters before the first interior capital.
func leadingVerb(opID string) string {
	for i, r := range opID {
		if i > 0 && unicode.IsUpper(r) {
			return opID[:i]
		}
	}
	return opID
}

// kebab lowercases and hyphenates a tag/param: handles "::", "_", spaces, and
// camelCase boundaries. "Orders::Invoice" -> "orders-invoice",
// "workspace_id" -> "workspace-id".
func kebab(s string) string {
	var b []rune
	prevAlnum := false
	for _, r := range s {
		switch {
		case r == ':' || r == '_' || r == ' ' || r == '/':
			if len(b) > 0 && b[len(b)-1] != '-' {
				b = append(b, '-')
			}
			prevAlnum = false
		case unicode.IsUpper(r):
			if prevAlnum && len(b) > 0 && b[len(b)-1] != '-' {
				b = append(b, '-')
			}
			b = append(b, unicode.ToLower(r))
			prevAlnum = true
		default:
			b = append(b, r)
			prevAlnum = unicode.IsLower(r) || unicode.IsDigit(r)
		}
	}
	return strings.Trim(string(b), "-")
}
