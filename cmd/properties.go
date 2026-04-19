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
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	PropertyType      *string `json:"property_type"`
	Status            *string `json:"status"`
	City              *string `json:"city"`
	Country           *string `json:"country"`
	MeasurementSystem string  `json:"measurement_system"`
	ReportingCurrency string  `json:"reporting_currency"`
}

// dateEntry is used by `kestrel properties date-entries <id>` — the full date
// dependency graph across KeyDate / LeaseDate / ExpenseDate / IncreaseDate /
// LeaseSecurityDate. Not paginated.
type dateEntry struct {
	ID            int     `json:"id"`
	Date          *string `json:"date"`
	EntryableType string  `json:"entryable_type"`
	DateableType  string  `json:"dateable_type"`
	DateableID    int     `json:"dateable_id"`
	Name          string  `json:"name"`
	FieldName     *string `json:"field_name"`
	Computed      bool    `json:"computed"`
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

// Drill-down commands: kestrel properties <subresource> <property-id>

var propertiesLeasesPage int
var propertiesLeasesCmd = &cobra.Command{
	Use:   "leases <property-id>",
	Short: "List leases on a property",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if propertiesLeasesPage > 1 {
			params["page"] = strconv.Itoa(propertiesLeasesPage)
		}
		raw, err := client.GetRaw("/properties/"+args[0]+"/leases", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []lease `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Name", "Status", "Start", "End"}
		rows := make([][]string, len(resp.Data))
		for i, l := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(l.ID),
				deref(l.Name),
				deref(l.SystemStatus),
				l.StartDate,
				l.EndDate,
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

var propertiesExpensesPage int
var propertiesExpensesCmd = &cobra.Command{
	Use:   "expenses <property-id>",
	Short: "List property-level expenses",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if propertiesExpensesPage > 1 {
			params["page"] = strconv.Itoa(propertiesExpensesPage)
		}
		raw, err := client.GetRaw("/properties/"+args[0]+"/expenses", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []expense `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Type", "Name", "Amount", "Currency", "Frequency", "Start", "End"}
		rows := make([][]string, len(resp.Data))
		for i, e := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(e.ID),
				e.ExpenseType,
				e.DisplayName,
				fmt.Sprintf("%.2f", e.Amount),
				e.AmountCurrency,
				e.Frequency,
				e.StartDate,
				e.EndDate,
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

var propertiesDocumentsPage int
var propertiesDocumentsCmd = &cobra.Command{
	Use:   "documents <property-id>",
	Short: "List documents linked to a property",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if propertiesDocumentsPage > 1 {
			params["page"] = strconv.Itoa(propertiesDocumentsPage)
		}
		raw, err := client.GetRaw("/properties/"+args[0]+"/documents", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []document `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Name", "Category", "Date", "Versions"}
		rows := make([][]string, len(resp.Data))
		for i, d := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(d.ID),
				d.Name,
				deref(d.Category1),
				deref(d.DocumentDate),
				strconv.Itoa(d.VersionCount),
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

var propertiesKeyDatesPage int
var propertiesKeyDatesCmd = &cobra.Command{
	Use:   "key-dates <property-id>",
	Short: "List property-level key dates",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if propertiesKeyDatesPage > 1 {
			params["page"] = strconv.Itoa(propertiesKeyDatesPage)
		}
		raw, err := client.GetRaw("/properties/"+args[0]+"/key_dates", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []keyDate `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Name", "Date", "Category", "Status", "Notice"}
		rows := make([][]string, len(resp.Data))
		for i, k := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(k.ID),
				k.Name,
				deref(k.Date),
				deref(k.Category1),
				deref(k.Status),
				derefBool(k.NoticeDeadline),
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

var propertiesDateEntriesCmd = &cobra.Command{
	Use:     "date-entries <property-id>",
	Aliases: []string{"date_entries"},
	Short:   "List the full date dependency graph for a property",
	Long: `Returns KeyDate, LeaseDate, ExpenseDate, IncreaseDate, and LeaseSecurityDate
entries with their dependency relationships. Not paginated — the full set is returned.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/properties/"+args[0]+"/date_entries", nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []dateEntry `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Entry type", "Name", "Field", "Date", "Parent", "Computed"}
		rows := make([][]string, len(resp.Data))
		for i, d := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(d.ID),
				d.EntryableType,
				d.Name,
				deref(d.FieldName),
				deref(d.Date),
				fmt.Sprintf("%s #%d", d.DateableType, d.DateableID),
				derefBool(&d.Computed),
			}
		}
		printer.Table(headers, rows)
		return nil
	},
}

func init() {
	propertiesListCmd.Flags().IntVar(&propertiesPage, "page", 1, "Page number")
	propertiesLeasesCmd.Flags().IntVar(&propertiesLeasesPage, "page", 1, "Page number")
	propertiesExpensesCmd.Flags().IntVar(&propertiesExpensesPage, "page", 1, "Page number")
	propertiesDocumentsCmd.Flags().IntVar(&propertiesDocumentsPage, "page", 1, "Page number")
	propertiesKeyDatesCmd.Flags().IntVar(&propertiesKeyDatesPage, "page", 1, "Page number")

	propertiesCmd.AddCommand(propertiesListCmd)
	propertiesCmd.AddCommand(propertiesShowCmd)
	propertiesCmd.AddCommand(propertiesLeasesCmd)
	propertiesCmd.AddCommand(propertiesExpensesCmd)
	propertiesCmd.AddCommand(propertiesDocumentsCmd)
	propertiesCmd.AddCommand(propertiesKeyDatesCmd)
	propertiesCmd.AddCommand(propertiesDateEntriesCmd)
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
