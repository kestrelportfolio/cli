package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// property matches the JSON shape from the API.
// In Ruby this would be a Struct or an OpenStruct. In Go, we define the
// exact fields we care about with json tags that map to the API field names.
type property struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	PropertyType     *string `json:"property_type"`
	Status           *string `json:"status"`
	City             *string `json:"city"`
	Country          *string `json:"country"`
	MeasurementSystem string `json:"measurement_system"`
	ReportingCurrency string `json:"reporting_currency"`
}

var propertiesPage int

var propertiesCmd = &cobra.Command{
	Use:   "properties",
	Short: "Manage properties",
}

var propertiesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List properties",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Token == "" {
			return fmt.Errorf("not logged in. Run: kestrel login")
		}

		params := map[string]string{}
		if propertiesPage > 1 {
			params["page"] = strconv.Itoa(propertiesPage)
		}

		raw, err := client.GetRaw("/properties", params)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}

		// Parse for table output
		var resp struct {
			Data []property          `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		headers := []string{"ID", "Name", "Type", "Status", "City", "Country"}
		rows := make([][]string, len(resp.Data))
		for i, p := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(p.ID),
				p.Name,
				deref(p.PropertyType),
				deref(p.Status),
				deref(p.City),
				deref(p.Country),
			}
		}

		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}

		return nil
	},
}

var propertiesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a property",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Token == "" {
			return fmt.Errorf("not logged in. Run: kestrel login")
		}

		raw, err := client.GetRaw("/properties/"+args[0], nil)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}

		var resp struct {
			Data property `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		p := resp.Data
		printer.Detail([][]string{
			{"ID", strconv.Itoa(p.ID)},
			{"Name", p.Name},
			{"Type", deref(p.PropertyType)},
			{"Status", deref(p.Status)},
			{"City", deref(p.City)},
			{"Country", deref(p.Country)},
			{"Measurement", p.MeasurementSystem},
			{"Currency", p.ReportingCurrency},
		})

		return nil
	},
}

func init() {
	propertiesListCmd.Flags().IntVar(&propertiesPage, "page", 1, "Page number")
	propertiesCmd.AddCommand(propertiesListCmd)
	propertiesCmd.AddCommand(propertiesShowCmd)
	rootCmd.AddCommand(propertiesCmd)
}

// deref safely dereferences a *string, returning "" if nil.
// Go doesn't have Ruby's &. (safe navigation) or JS's ?. (optional chaining),
// so we use a helper for nullable string fields.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
