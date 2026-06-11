package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/clickfunnels/clickfunnels-cli/internal/api"
	"github.com/clickfunnels/clickfunnels-cli/internal/output"
	"github.com/clickfunnels/clickfunnels-cli/internal/ui"
)

func newContactsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contacts",
		Short: "Manage contacts",
	}
	cmd.AddCommand(newContactsListCmd())
	cmd.AddCommand(newContactsCreateCmd())
	cmd.AddCommand(newContactsUpdateCmd())
	cmd.AddCommand(newContactsDeleteCmd())
	return cmd
}

func contactColumns() []output.Column[api.ContactAttributes] {
	return []output.Column[api.ContactAttributes]{
		{Header: "ID", Value: func(c api.ContactAttributes) string { return c.PublicId }},
		{Header: "EMAIL", Value: func(c api.ContactAttributes) string { return c.EmailAddress }},
		{Header: "NAME", Value: func(c api.ContactAttributes) string { return contactName(c) }},
	}
}

func newContactsListCmd() *cobra.Command {
	var after string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, account, err := authedClient(cmd)
			if err != nil {
				return err
			}

			params := &api.ListContactsParams{}
			if after != "" {
				a := api.After(after)
				params.After = &a
			}
			resp, err := client.ListContactsWithResponse(cmd.Context(), int(account.WorkspaceID), params)
			if err != nil {
				return err
			}
			contacts := derefSlice(resp.JSON200)

			if format == output.Table && len(contacts) == 0 {
				fmt.Println(ui.Subtle.Render("No contacts found."))
				return nil
			}
			if err := output.Collection(os.Stdout, format, contactColumns(), contacts); err != nil {
				return err
			}
			if format == output.Table {
				if next := api.Cursor(resp.HTTPResponse); next != "" {
					fmt.Println(ui.Subtle.Render(fmt.Sprintf("\nMore results — fetch with: cf contacts list --after %s", next)))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	return cmd
}

// contactFlags holds the writable-field flags shared by create and update.
type contactFlags struct {
	email, first, last, phone string
	tagIDs                    []int
	attrs                     map[string]string
	input                     string
}

func (cf *contactFlags) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&cf.email, "email", "", "email address")
	f.StringVar(&cf.first, "first-name", "", "first name")
	f.StringVar(&cf.last, "last-name", "", "last name")
	f.StringVar(&cf.phone, "phone", "", "phone number")
	f.IntSliceVar(&cf.tagIDs, "tag-id", nil, "tag id to apply (repeatable); overwrites existing tags")
	f.StringToStringVar(&cf.attrs, "attr", nil, "custom attribute key=value (repeatable)")
	f.StringVar(&cf.input, "input", "", "read full contact JSON from a file, or '-' for stdin")
}

// build assembles ContactParameters from --input (if any) overlaid with
// explicitly set flags. The bool reports whether anything was provided.
func (cf *contactFlags) build(cmd *cobra.Command) (api.ContactParameters, bool, error) {
	var p api.ContactParameters
	provided := false

	if cf.input != "" {
		data, err := readInput(cf.input)
		if err != nil {
			return p, false, err
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return p, false, fmt.Errorf("parsing --input JSON: %w", err)
		}
		provided = true
	}

	f := cmd.Flags()
	if f.Changed("email") {
		p.EmailAddress, provided = &cf.email, true
	}
	if f.Changed("first-name") {
		p.FirstName, provided = &cf.first, true
	}
	if f.Changed("last-name") {
		p.LastName, provided = &cf.last, true
	}
	if f.Changed("phone") {
		p.PhoneNumber, provided = &cf.phone, true
	}
	if f.Changed("tag-id") {
		tags := cf.tagIDs
		p.TagIds, provided = &tags, true
	}
	if f.Changed("attr") {
		attrs := cf.attrs
		p.CustomAttributes, provided = &attrs, true
	}
	return p, provided, nil
}

func newContactsCreateCmd() *cobra.Command {
	var cf contactFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact",
		Long: "Create a contact. Pass field flags, supply a full JSON body with " +
			"--input, or run with no arguments for an interactive form.",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, account, err := authedClient(cmd)
			if err != nil {
				return err
			}

			params, provided, err := cf.build(cmd)
			if err != nil {
				return err
			}
			if !provided {
				form := huh.NewForm(huh.NewGroup(
					huh.NewInput().Title("Email address").Value(&cf.email),
					huh.NewInput().Title("First name").Value(&cf.first),
					huh.NewInput().Title("Last name").Value(&cf.last),
					huh.NewInput().Title("Phone number").Value(&cf.phone),
				))
				if err := form.Run(); err != nil {
					return err
				}
				params = api.ContactParameters{
					EmailAddress: nonEmpty(cf.email),
					FirstName:    nonEmpty(cf.first),
					LastName:     nonEmpty(cf.last),
					PhoneNumber:  nonEmpty(cf.phone),
				}
			}

			body := api.CreateContactsJSONRequestBody{Contact: &params}
			resp, err := client.CreateContactsWithResponse(cmd.Context(), int(account.WorkspaceID), &api.CreateContactsParams{}, body)
			if err != nil {
				return err
			}
			if format != output.Table {
				return output.Object(os.Stdout, format, resp.JSON201)
			}
			fmt.Printf("%s Created contact %s %s\n",
				ui.Success.Render(ui.Check),
				ui.Accent.Render(resp.JSON201.PublicId),
				ui.Subtle.Render(resp.JSON201.EmailAddress))
			return nil
		},
	}
	cf.bind(cmd)
	return cmd
}

func newContactsUpdateCmd() *cobra.Command {
	var cf contactFlags
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a contact by id",
		Long:  "Update a contact. Only the fields you pass (flags or --input) are sent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			params, provided, err := cf.build(cmd)
			if err != nil {
				return err
			}
			if !provided {
				return fmt.Errorf("nothing to update — pass a field flag, or --input")
			}

			update := api.ToContactUpdate(params)
			body := api.UpdateContactsJSONRequestBody{Contact: &update}
			resp, err := client.UpdateContactsWithResponse(cmd.Context(), api.Id(args[0]), &api.UpdateContactsParams{}, body)
			if err != nil {
				return err
			}
			if format != output.Table {
				return output.Object(os.Stdout, format, resp.JSON200)
			}
			fmt.Printf("%s Updated contact %s\n", ui.Success.Render(ui.Check), ui.Accent.Render(resp.JSON200.PublicId))
			return nil
		},
	}
	cf.bind(cmd)
	return cmd
}

func newContactsDeleteCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a contact by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			id := args[0]

			if !force {
				// No interactive prompt in non-table (scripting) modes.
				if format != output.Table {
					return fmt.Errorf("refusing to delete without --force when --output=%s", format)
				}
				confirm := false
				err := huh.NewForm(huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Delete contact %s?", id)).
						Description("This cannot be undone.").
						Value(&confirm),
				)).Run()
				if err != nil {
					return err
				}
				if !confirm {
					fmt.Println(ui.Subtle.Render("Aborted."))
					return nil
				}
			}

			if _, err := client.RemoveContactsWithResponse(cmd.Context(), api.Id(id)); err != nil {
				return err
			}
			if format != output.Table {
				return output.Object(os.Stdout, format, map[string]any{"id": id, "deleted": true})
			}
			fmt.Printf("%s Deleted contact %s\n", ui.Success.Render(ui.Check), ui.Accent.Render(id))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip the confirmation prompt")
	return cmd
}

// --- small shared helpers ---

// readInput reads from a file path, or from stdin when path is "-".
func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// nonEmpty returns a pointer to s, or nil if empty, so omitempty drops it.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// str dereferences an optional string field from a generated model.
func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// derefSlice returns the slice a generated *[]T points to, or nil.
func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

// contactName builds a display name from a contact's first/last name.
func contactName(c api.ContactAttributes) string {
	name := c.FirstName
	if c.LastName != "" {
		if name != "" {
			name += " "
		}
		name += c.LastName
	}
	return name
}
