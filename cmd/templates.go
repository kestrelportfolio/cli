package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type abstractionTemplate struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Kind        *string `json:"kind"`
	IsDefault   bool    `json:"is_default"`
	Requirements []abstractionRequirement `json:"requirements,omitempty"`
}

type abstractionRequirement struct {
	ID                 int     `json:"id"`
	Kind               string  `json:"kind"`
	TargetModel        string  `json:"target_model"`
	TargetField        *string `json:"target_field"`
	MinCount           *int    `json:"min_count"`
	Condition          *string `json:"condition"`
	Section            *string `json:"section"`
	Position           int     `json:"position"`
	RequiredForApproval bool   `json:"required_for_approval"`
}

// schemaFieldSpec is one field in the authoring schema.
type schemaFieldSpec struct {
	FieldName    string          `json:"field_name"`
	Type         string          `json:"type"`
	Caption      *string         `json:"caption"`
	Required     bool            `json:"required"`
	Options      json.RawMessage `json:"options"`
	DefaultValue json.RawMessage `json:"default_value"`
	Guidance     *string         `json:"guidance"`
}

// schemaSubObject represents a sub-object requirement group.
type schemaSubObject struct {
	Kind      string            `json:"kind"`
	MinCount  *int              `json:"min_count"`
	Condition *string           `json:"condition"`
	Fields    []schemaFieldSpec `json:"fields"`
}

// schemaModel is the per-model shape returned by /schema endpoints.
type schemaModel struct {
	Primary    bool              `json:"primary"`
	Fields     []schemaFieldSpec `json:"fields"`
	SubObjects []schemaSubObject `json:"sub_objects"`
}

var templatesCmd = &cobra.Command{
	Use:     "templates",
	Aliases: []string{"abstraction-templates", "abstraction_templates"},
	Short:   "Browse abstraction templates",
}

var templatesListPage int
var templatesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available abstraction templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if templatesListPage > 1 {
			params["page"] = strconv.Itoa(templatesListPage)
		}
		raw, err := client.GetRaw("/abstraction_templates", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []abstractionTemplate `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Name", "Kind", "Default", "Description"}
		rows := make([][]string, len(resp.Data))
		for i, t := range resp.Data {
			def := ""
			if t.IsDefault {
				def = "yes"
			}
			rows[i] = []string{
				strconv.Itoa(t.ID),
				t.Name,
				deref(t.Kind),
				def,
				deref(t.Description),
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		printer.Breadcrumb("Preview schema: kestrel templates schema <id>")
		return nil
	},
}

var templatesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a template with its ordered requirements",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/abstraction_templates/"+args[0], nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data abstractionTemplate `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		t := resp.Data
		printer.Detail([][]string{
			{"ID", strconv.Itoa(t.ID)},
			{"Name", t.Name},
			{"Kind", deref(t.Kind)},
			{"Default", derefBool(&t.IsDefault)},
			{"Description", deref(t.Description)},
		})
		if len(t.Requirements) > 0 {
			fmt.Println()
			fmt.Println("Requirements")
			fmt.Println(strings.Repeat("─", 12))
			headers := []string{"Pos", "Kind", "Model", "Field", "Min", "Section", "Approval"}
			rows := make([][]string, len(t.Requirements))
			for i, r := range t.Requirements {
				rows[i] = []string{
					strconv.Itoa(r.Position),
					r.Kind,
					r.TargetModel,
					deref(r.TargetField),
					derefInt(r.MinCount),
					deref(r.Section),
					derefBool(&r.RequiredForApproval),
				}
			}
			printer.Table(headers, rows)
		}
		printer.Breadcrumb(fmt.Sprintf("Preview authoring schema: kestrel templates schema %s", args[0]))
		printer.Breadcrumb(fmt.Sprintf("Start an abstraction: kestrel abstractions create --template-id %s --kind greenfield", args[0]))
		return nil
	},
}

var templatesSchemaCmd = &cobra.Command{
	Use:   "schema <id>",
	Short: "Preview the authoring schema a template produces",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/abstraction_templates/"+args[0]+"/schema", nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		return renderAbstractionSchema(raw)
	},
}

// renderAbstractionSchema prints a schema response as per-model sections.
// Shared by `templates schema` and `abstractions schema`.
func renderAbstractionSchema(raw []byte) error {
	var resp struct {
		Data struct {
			Models map[string]schemaModel `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing schema: %w", err)
	}

	// Primary models (Property, Lease) first, then alphabetical.
	keys := make([]string, 0, len(resp.Data.Models))
	for k := range resp.Data.Models {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		pi, pj := resp.Data.Models[keys[i]].Primary, resp.Data.Models[keys[j]].Primary
		if pi != pj {
			return pi
		}
		return keys[i] < keys[j]
	})

	for i, k := range keys {
		m := resp.Data.Models[k]
		if i > 0 {
			fmt.Println()
		}
		title := k
		if m.Primary {
			title += "  (primary)"
		}
		fmt.Println(title)
		fmt.Println(strings.Repeat("─", len(title)))
		if len(m.Fields) > 0 {
			headers := []string{"Field", "Type", "Caption", "Required", "Options", "Default"}
			rows := make([][]string, len(m.Fields))
			for j, f := range m.Fields {
				rows[j] = []string{
					f.FieldName,
					f.Type,
					deref(f.Caption),
					derefBool(&f.Required),
					previewJSON(f.Options),
					previewJSON(f.DefaultValue),
				}
			}
			printer.Table(headers, rows)
		}
		for _, so := range m.SubObjects {
			label := "  sub-object"
			if so.MinCount != nil {
				label += fmt.Sprintf(" (min %d)", *so.MinCount)
			}
			if so.Condition != nil && *so.Condition != "" {
				label += fmt.Sprintf(" [condition: %s]", *so.Condition)
			}
			fmt.Println(label)
			if len(so.Fields) == 0 {
				continue
			}
			headers := []string{"Field", "Type", "Caption", "Required"}
			rows := make([][]string, len(so.Fields))
			for j, f := range so.Fields {
				rows[j] = []string{
					"  " + f.FieldName,
					f.Type,
					deref(f.Caption),
					derefBool(&f.Required),
				}
			}
			printer.Table(headers, rows)
		}
	}
	return nil
}

// previewJSON renders a json.RawMessage as a single-line string, or "" for null/empty.
func previewJSON(r json.RawMessage) string {
	s := strings.TrimSpace(string(r))
	if s == "" || s == "null" {
		return ""
	}
	return s
}

func init() {
	templatesListCmd.Flags().IntVar(&templatesListPage, "page", 1, "Page number")
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesShowCmd)
	templatesCmd.AddCommand(templatesSchemaCmd)
	rootCmd.AddCommand(templatesCmd)
}
