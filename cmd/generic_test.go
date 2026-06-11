package cmd

import (
	"reflect"
	"testing"
)

func TestKebab(t *testing.T) {
	cases := map[string]string{
		"Orders::Invoice":              "orders-invoice",
		"workspace_id":                 "workspace-id",
		"Contact":                      "contact",
		"Blogs::Post":                  "blogs-post",
		"Appointments::ScheduledEvent": "appointments-scheduled-event",
		"Forms::Fields::Option":        "forms-fields-option",
	}
	for in, want := range cases {
		if got := kebab(in); got != want {
			t.Errorf("kebab(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLeadingVerb(t *testing.T) {
	cases := map[string]string{
		"listOrdersInvoices":   "list",
		"createBlogPost":       "create",
		"getContacts":          "get",
		"gdpr_destroyContacts": "gdpr_destroy",
		"upsertContacts":       "upsert",
	}
	for in, want := range cases {
		if got := leadingVerb(in); got != want {
			t.Errorf("leadingVerb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathParams(t *testing.T) {
	got := pathParams("/workspaces/{workspace_id}/orders/{order_id}/invoices")
	want := []string{"workspace_id", "order_id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pathParams = %v, want %v", got, want)
	}
	if len(pathParams("/teams")) != 0 {
		t.Errorf("expected no params for /teams")
	}
}

// TestManifestNonEmpty guards against a broken generation step.
func TestManifestNonEmpty(t *testing.T) {
	if len(generatedOperations) < 200 {
		t.Errorf("expected the full operation manifest (~262), got %d", len(generatedOperations))
	}
	for _, o := range generatedOperations {
		if o.OpID == "" || o.Method == "" || o.Path == "" {
			t.Fatalf("malformed operation in manifest: %+v", o)
		}
	}
}
