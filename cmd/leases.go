package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

type lease struct {
	ID           int     `json:"id"`
	Name         *string `json:"name"`
	Status       *string `json:"status"`
	SystemStatus *string `json:"system_status"`
	PropertyID   int     `json:"property_id"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
}

// Sub-resource types used by lease + property drill-downs.

type keyDate struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Date           *string `json:"date"`
	NoticeDeadline *bool   `json:"notice_deadline"`
	Category1      *string `json:"category_1"`
	Category2      *string `json:"category_2"`
	Status         *string `json:"status"`
	DateableType   string  `json:"dateable_type"`
	DateableID     int     `json:"dateable_id"`
}

type leaseComponentArea struct {
	ID       int      `json:"id"`
	LeaseID  int      `json:"lease_id"`
	AreaType string   `json:"area_type"`
	AreaSqm  *float64 `json:"area_sqm"`
	AreaSqft *float64 `json:"area_sqft"`
	Comment  *string  `json:"comment"`
}

type leaseClause struct {
	ID        int     `json:"id"`
	LeaseID   int     `json:"lease_id"`
	Name      string  `json:"name"`
	Category1 *string `json:"category_1"`
	Category2 *string `json:"category_2"`
}

var leasesPage int

var leasesCmd = &cobra.Command{
	Use:   "leases",
	Short: "Manage leases",
}

var leasesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List leases",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesPage > 1 {
			params["page"] = strconv.Itoa(leasesPage)
		}
		raw, err := client.GetRaw("/leases", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []lease        `json:"data"`
				Meta *paginatedMeta `json:"meta"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			headers := []string{"ID", "Name", "Property", "Status", "Start", "End"}
			rows := make([][]string, len(resp.Data))
			for i, l := range resp.Data {
				rows[i] = []string{
					strconv.Itoa(l.ID),
					deref(l.Name),
					strconv.Itoa(l.PropertyID),
					deref(l.SystemStatus),
					l.StartDate,
					l.EndDate,
				}
			}
			printer.Table(headers, rows)
			if resp.Meta != nil {
				printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var leasesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a lease",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/leases/"+args[0], nil)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data lease `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			l := resp.Data
			printer.Detail([][]string{
				{"ID", strconv.Itoa(l.ID)},
				{"Name", deref(l.Name)},
				{"Property ID", strconv.Itoa(l.PropertyID)},
				{"Status", deref(l.Status)},
				{"System status", deref(l.SystemStatus)},
				{"Start date", l.StartDate},
				{"End date", l.EndDate},
			})
		}
		printer.FinishRaw(raw)
		return nil
	},
}

// Drill-down commands: kestrel leases <subresource> <lease-id>
// Each mirrors a nested route under /leases/:id.

var leasesExpensesPage int
var leasesExpensesCmd = &cobra.Command{
	Use:   "expenses <lease-id>",
	Short: "List expenses attached to a lease",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesExpensesPage > 1 {
			params["page"] = strconv.Itoa(leasesExpensesPage)
		}
		raw, err := client.GetRaw("/leases/"+args[0]+"/expenses", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []expense      `json:"data"`
				Meta *paginatedMeta `json:"meta"`
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
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var leasesDocumentsPage int
var leasesDocumentsCmd = &cobra.Command{
	Use:   "documents <lease-id>",
	Short: "List documents linked to a lease",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesDocumentsPage > 1 {
			params["page"] = strconv.Itoa(leasesDocumentsPage)
		}
		raw, err := client.GetRaw("/leases/"+args[0]+"/documents", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []document     `json:"data"`
				Meta *paginatedMeta `json:"meta"`
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
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var leasesKeyDatesPage int
var leasesKeyDatesCmd = &cobra.Command{
	Use:   "key-dates <lease-id>",
	Short: "List lease-level key dates",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesKeyDatesPage > 1 {
			params["page"] = strconv.Itoa(leasesKeyDatesPage)
		}
		raw, err := client.GetRaw("/leases/"+args[0]+"/key_dates", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []keyDate      `json:"data"`
				Meta *paginatedMeta `json:"meta"`
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
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var leasesComponentAreasPage int
var leasesComponentAreasCmd = &cobra.Command{
	Use:     "component-areas <lease-id>",
	Aliases: []string{"component_areas"},
	Short:   "List component areas for a lease",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesComponentAreasPage > 1 {
			params["page"] = strconv.Itoa(leasesComponentAreasPage)
		}
		raw, err := client.GetRaw("/leases/"+args[0]+"/component_areas", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []leaseComponentArea `json:"data"`
				Meta *paginatedMeta       `json:"meta"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			headers := []string{"ID", "Type", "Area (sqm)", "Area (sqft)", "Comment"}
			rows := make([][]string, len(resp.Data))
			for i, c := range resp.Data {
				rows[i] = []string{
					strconv.Itoa(c.ID),
					c.AreaType,
					derefFloat(c.AreaSqm),
					derefFloat(c.AreaSqft),
					deref(c.Comment),
				}
			}
			printer.Table(headers, rows)
			if resp.Meta != nil {
				printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var leasesSecuritiesPage int
var leasesSecuritiesCmd = &cobra.Command{
	Use:   "securities <lease-id>",
	Short: "List lease securities (deposits, guarantees, LOCs) for a lease",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesSecuritiesPage > 1 {
			params["page"] = strconv.Itoa(leasesSecuritiesPage)
		}
		raw, err := client.GetRaw("/leases/"+args[0]+"/securities", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []leaseSecurity `json:"data"`
				Meta *paginatedMeta  `json:"meta"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			headers := []string{"ID", "Type", "Status", "Amount", "Currency", "Required", "Returned"}
			rows := make([][]string, len(resp.Data))
			for i, s := range resp.Data {
				rows[i] = []string{
					strconv.Itoa(s.ID),
					s.SecurityType,
					deref(s.Status),
					derefFloat(s.Amount),
					deref(s.AmountCurrency),
					deref(s.RequiredDate),
					deref(s.ReturnDate),
				}
			}
			printer.Table(headers, rows)
			if resp.Meta != nil {
				printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var leasesClausesPage int
var leasesClausesCmd = &cobra.Command{
	Use:   "clauses <lease-id>",
	Short: "List lease clauses",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if leasesClausesPage > 1 {
			params["page"] = strconv.Itoa(leasesClausesPage)
		}
		raw, err := client.GetRaw("/leases/"+args[0]+"/lease_clauses", params)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []leaseClause  `json:"data"`
				Meta *paginatedMeta `json:"meta"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			headers := []string{"ID", "Name", "Category 1", "Category 2"}
			rows := make([][]string, len(resp.Data))
			for i, c := range resp.Data {
				rows[i] = []string{
					strconv.Itoa(c.ID),
					c.Name,
					deref(c.Category1),
					deref(c.Category2),
				}
			}
			printer.Table(headers, rows)
			if resp.Meta != nil {
				printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

func init() {
	leasesListCmd.Flags().IntVar(&leasesPage, "page", 1, "Page number")
	leasesExpensesCmd.Flags().IntVar(&leasesExpensesPage, "page", 1, "Page number")
	leasesDocumentsCmd.Flags().IntVar(&leasesDocumentsPage, "page", 1, "Page number")
	leasesKeyDatesCmd.Flags().IntVar(&leasesKeyDatesPage, "page", 1, "Page number")
	leasesComponentAreasCmd.Flags().IntVar(&leasesComponentAreasPage, "page", 1, "Page number")
	leasesSecuritiesCmd.Flags().IntVar(&leasesSecuritiesPage, "page", 1, "Page number")
	leasesClausesCmd.Flags().IntVar(&leasesClausesPage, "page", 1, "Page number")

	leasesCmd.AddCommand(leasesListCmd)
	leasesCmd.AddCommand(leasesShowCmd)
	leasesCmd.AddCommand(leasesExpensesCmd)
	leasesCmd.AddCommand(leasesDocumentsCmd)
	leasesCmd.AddCommand(leasesKeyDatesCmd)
	leasesCmd.AddCommand(leasesComponentAreasCmd)
	leasesCmd.AddCommand(leasesSecuritiesCmd)
	leasesCmd.AddCommand(leasesClausesCmd)
	rootCmd.AddCommand(leasesCmd)
}
