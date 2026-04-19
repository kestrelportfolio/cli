package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type fieldConfigEntry struct {
	FieldName      string   `json:"field_name"`
	Type           string   `json:"type"`
	Caption        *string  `json:"caption"`
	Required       bool     `json:"required"`
	AlwaysRequired bool     `json:"always_required"`
	Options        []string `json:"options"`
}

var fieldConfigsCmd = &cobra.Command{
	Use:     "field-configs",
	Aliases: []string{"field_configs"},
	Short:   "Organization field configuration (captions, required, options)",
}

var fieldConfigsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List field configurations grouped by model",
	Long:  `Returns the org's field labels, required flags, and dropdown options for all configurable models. Use this to discover what extra_* fields are surfaced on Property, Lease, and Expense responses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}

		raw, err := client.GetRaw("/field_configs", nil)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}

		var resp struct {
			Data map[string][]fieldConfigEntry `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		// Sort model keys for stable, scannable output.
		keys := make([]string, 0, len(resp.Data))
		for k := range resp.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for i, k := range keys {
			if i > 0 {
				fmt.Println()
			}
			fmt.Println(k)
			fmt.Println(strings.Repeat("─", len(k)))
			headers := []string{"Field", "Type", "Caption", "Required", "Options"}
			rows := make([][]string, len(resp.Data[k]))
			for j, e := range resp.Data[k] {
				req := "no"
				if e.AlwaysRequired {
					req = "always"
				} else if e.Required {
					req = "yes"
				}
				rows[j] = []string{
					e.FieldName,
					e.Type,
					deref(e.Caption),
					req,
					strings.Join(e.Options, ", "),
				}
			}
			printer.Table(headers, rows)
		}

		return nil
	},
}

func init() {
	fieldConfigsCmd.AddCommand(fieldConfigsListCmd)
	rootCmd.AddCommand(fieldConfigsCmd)
}
