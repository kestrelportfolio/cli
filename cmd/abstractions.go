package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/spf13/cobra"
)

type abstraction struct {
	ID                   int     `json:"id"`
	Name                 string  `json:"name"`
	Kind                 string  `json:"kind"`
	State                string  `json:"state"`
	TemplateID           *int    `json:"template_id"`
	TemplateName         *string `json:"template_name"`
	TargetPropertyID     *int    `json:"target_property_id"`
	TargetLeaseID        *int    `json:"target_lease_id"`
	SubmittedAt          *string `json:"submitted_at"`
	CompletedAt          *string `json:"completed_at"`
	ChangesCount         *int    `json:"changes_count,omitempty"`
	SourceDocumentsCount *int    `json:"source_documents_count,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

var abstractionsCmd = &cobra.Command{
	Use:     "abstractions",
	Aliases: []string{"abstraction"},
	Short:   "Author abstractions — the lease data authoring workflow",
	Long: `Abstractions are the write surface for the Kestrel API. Each abstraction
produces staged changes against Property, Lease, and sub-objects like KeyDate
and Expense. Once go-live happens (web UI), changes are applied to live tables.

Typical flow:
  kestrel templates list
  kestrel abstractions create --template-id N --kind greenfield
  kestrel abstractions schema <id>                    # discover what to fill
  kestrel abstractions add-doc <id> lease.pdf         # attach source document
  kestrel abstractions changes create <id> ...        # draft per-field changes
  (web UI: approve + go-live)`,
}

var abstractionsListPage int
var abstractionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List abstractions",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if abstractionsListPage > 1 {
			params["page"] = strconv.Itoa(abstractionsListPage)
		}
		raw, err := client.GetRaw("/abstractions", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []abstraction `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Name", "Template", "Kind", "State", "Property", "Lease"}
		rows := make([][]string, len(resp.Data))
		for i, a := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(a.ID),
				a.Name,
				deref(a.TemplateName),
				a.Kind,
				a.State,
				derefInt(a.TargetPropertyID),
				derefInt(a.TargetLeaseID),
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

var abstractionsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a single abstraction with change/doc counts",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/abstractions/"+args[0], nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data abstraction `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		a := resp.Data
		printer.Detail([][]string{
			{"ID", strconv.Itoa(a.ID)},
			{"Name", a.Name},
			{"Kind", a.Kind},
			{"State", a.State},
			{"Template ID", derefInt(a.TemplateID)},
			{"Template", deref(a.TemplateName)},
			{"Target property", derefInt(a.TargetPropertyID)},
			{"Target lease", derefInt(a.TargetLeaseID)},
			{"Submitted at", deref(a.SubmittedAt)},
			{"Completed at", deref(a.CompletedAt)},
			{"Changes", derefInt(a.ChangesCount)},
			{"Sources", derefInt(a.SourceDocumentsCount)},
			{"Created", a.CreatedAt},
			{"Updated", a.UpdatedAt},
		})
		if a.State == "in_progress" {
			printer.Breadcrumb(fmt.Sprintf("Schema: kestrel abstractions schema %d", a.ID))
			printer.Breadcrumb(fmt.Sprintf("Changes: kestrel abstractions changes list %d", a.ID))
			printer.Breadcrumb(fmt.Sprintf("Sources: kestrel abstractions sources %d", a.ID))
		}
		return nil
	},
}

var (
	abstractionsCreateTemplateID       int
	abstractionsCreateKind             string
	abstractionsCreateTargetPropertyID int
	abstractionsCreateTargetLeaseID    int
	abstractionsCreateName             string
)

var abstractionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new abstraction",
	Long: `Starts a new abstraction against a template.

  Greenfield — creates new Property/Lease on go-live. No target IDs needed.
  Brownfield — edits an existing lease. --target-property-id AND --target-lease-id are required.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		if abstractionsCreateTemplateID == 0 {
			return fmt.Errorf("--template-id is required")
		}
		if abstractionsCreateKind != "greenfield" && abstractionsCreateKind != "brownfield" {
			return fmt.Errorf("--kind must be 'greenfield' or 'brownfield'")
		}
		if abstractionsCreateKind == "brownfield" {
			if abstractionsCreateTargetPropertyID == 0 || abstractionsCreateTargetLeaseID == 0 {
				return fmt.Errorf("brownfield abstractions require --target-property-id AND --target-lease-id")
			}
		}

		attrs := map[string]any{
			"kind":                    abstractionsCreateKind,
			"abstraction_template_id": abstractionsCreateTemplateID,
		}
		if abstractionsCreateTargetPropertyID > 0 {
			attrs["target_property_id"] = abstractionsCreateTargetPropertyID
		}
		if abstractionsCreateTargetLeaseID > 0 {
			attrs["target_lease_id"] = abstractionsCreateTargetLeaseID
		}
		if abstractionsCreateName != "" {
			attrs["name"] = abstractionsCreateName
		}

		env, err := client.Post("/abstractions", map[string]any{"abstraction": attrs})
		if err != nil {
			return err
		}
		return renderAbstractionResult(env, true)
	},
}

var abstractionsUpdateName string
var abstractionsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update an abstraction (name only — state transitions use dedicated endpoints)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		if abstractionsUpdateName == "" {
			return fmt.Errorf("--name is required")
		}
		body := map[string]any{"abstraction": map[string]any{"name": abstractionsUpdateName}}
		env, err := client.Patch("/abstractions/"+args[0], body)
		if err != nil {
			return err
		}
		return renderAbstractionResult(env, false)
	},
}

var abstractionsAbandonCmd = &cobra.Command{
	Use:   "abandon <id>",
	Short: "Abandon an abstraction (irreversible — no changes are applied)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		env, err := client.Post("/abstractions/"+args[0]+"/abandon", nil)
		if err != nil {
			return err
		}
		return renderAbstractionResult(env, false)
	},
}

var abstractionsSchemaCmd = &cobra.Command{
	Use:   "schema <id>",
	Short: "Show the authoring schema for an abstraction",
	Long:  `Returns the merged field schema per model — what this abstraction is expected to fill in. Use this to discover valid target_type and target_field values for changes create.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/abstractions/"+args[0]+"/schema", nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		if err := renderAbstractionSchema(raw); err != nil {
			return err
		}
		printer.Breadcrumb(fmt.Sprintf("Draft a change: kestrel abstractions changes create %s --action update --target-type <Model> --payload '{\"field\":\"value\"}'", args[0]))
		return nil
	},
}

// renderAbstractionResult prints the envelope result of a create/update/abandon.
// If newlyCreated is true, it emits follow-up breadcrumbs (schema / sources).
func renderAbstractionResult(env *api.Envelope, newlyCreated bool) error {
	if env == nil || len(env.Data) == 0 {
		return nil
	}
	if printer.IsJSON() {
		pretty, err := json.MarshalIndent(env, "", "  ")
		if err == nil {
			fmt.Println(string(pretty))
		}
		return nil
	}
	var a abstraction
	if err := json.Unmarshal(env.Data, &a); err != nil {
		return fmt.Errorf("parsing abstraction: %w", err)
	}
	printer.Detail([][]string{
		{"ID", strconv.Itoa(a.ID)},
		{"Name", a.Name},
		{"Kind", a.Kind},
		{"State", a.State},
		{"Template", deref(a.TemplateName)},
		{"Target property", derefInt(a.TargetPropertyID)},
		{"Target lease", derefInt(a.TargetLeaseID)},
	})
	if newlyCreated {
		printer.Success(fmt.Sprintf("Abstraction #%d created", a.ID))
		printer.Breadcrumb(fmt.Sprintf("Discover fields: kestrel abstractions schema %d", a.ID))
		printer.Breadcrumb(fmt.Sprintf("Add source doc: kestrel abstractions add-doc %d <file>", a.ID))
	}
	return nil
}

func init() {
	abstractionsListCmd.Flags().IntVar(&abstractionsListPage, "page", 1, "Page number")

	abstractionsCreateCmd.Flags().IntVar(&abstractionsCreateTemplateID, "template-id", 0, "Template ID (required)")
	abstractionsCreateCmd.Flags().StringVar(&abstractionsCreateKind, "kind", "", "greenfield | brownfield (required)")
	abstractionsCreateCmd.Flags().IntVar(&abstractionsCreateTargetPropertyID, "target-property-id", 0, "Target property ID (brownfield only)")
	abstractionsCreateCmd.Flags().IntVar(&abstractionsCreateTargetLeaseID, "target-lease-id", 0, "Target lease ID (brownfield only)")
	abstractionsCreateCmd.Flags().StringVar(&abstractionsCreateName, "name", "", "Abstraction name (optional — auto-filled from template + property)")

	abstractionsUpdateCmd.Flags().StringVar(&abstractionsUpdateName, "name", "", "New abstraction name (required)")

	abstractionsCmd.AddCommand(abstractionsListCmd)
	abstractionsCmd.AddCommand(abstractionsShowCmd)
	abstractionsCmd.AddCommand(abstractionsCreateCmd)
	abstractionsCmd.AddCommand(abstractionsUpdateCmd)
	abstractionsCmd.AddCommand(abstractionsAbandonCmd)
	abstractionsCmd.AddCommand(abstractionsSchemaCmd)
	rootCmd.AddCommand(abstractionsCmd)
}
