package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

type expense struct {
	ID              int      `json:"id"`
	Name            *string  `json:"name"`
	DisplayName     string   `json:"display_name"`
	ExpenseType     string   `json:"expense_type"`
	Amount          float64  `json:"amount"`
	CurrentAmount   *float64 `json:"current_amount"`
	AmountCurrency  string   `json:"amount_currency"`
	Frequency       string   `json:"frequency"`
	PaymentTiming   string   `json:"payment_timing"`
	StartDate       string   `json:"start_date"`
	EndDate         string   `json:"end_date"`
	VendorName      *string  `json:"vendor_name"`
	ExpenseableType string   `json:"expenseable_type"`
	ExpenseableID   int      `json:"expenseable_id"`
	PropertyID      int      `json:"property_id"`
	LeaseID         *int     `json:"lease_id"`
}

type payment struct {
	ID             int     `json:"id"`
	ExpenseID      int     `json:"expense_id"`
	Amount         float64 `json:"amount"`
	AmountCurrency string  `json:"amount_currency"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	DueDate        string  `json:"due_date"`
	Status         string  `json:"status"`
	ApprovalStatus string  `json:"approval_status"`
}

// increase has no org-scoped id — it's identified by parent + effective_date.
type increase struct {
	IncreaseType      string   `json:"increase_type"`
	EffectiveDate     string   `json:"effective_date"`
	FixedAmount       *float64 `json:"fixed_amount"`
	PercentageValue   *float64 `json:"percentage_value"`
	ResolvedAmount    float64  `json:"resolved_amount"`
	StartIndexPeriod  *string  `json:"start_index_period"`
	EndIndexPeriod    *string  `json:"end_index_period"`
	AppliedPercentage *float64 `json:"applied_percentage"`
}

var expensesCmd = &cobra.Command{
	Use:   "expenses",
	Short: "Inspect expenses, their payments, and increases",
}

var expensesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a single expense",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/expenses/"+args[0], nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data expense `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		e := resp.Data
		printer.Detail([][]string{
			{"ID", strconv.Itoa(e.ID)},
			{"Name", deref(e.Name)},
			{"Display name", e.DisplayName},
			{"Type", e.ExpenseType},
			{"Amount", fmt.Sprintf("%.2f %s", e.Amount, e.AmountCurrency)},
			{"Current amount", derefFloat(e.CurrentAmount)},
			{"Frequency", e.Frequency},
			{"Payment timing", e.PaymentTiming},
			{"Start date", e.StartDate},
			{"End date", e.EndDate},
			{"Vendor", deref(e.VendorName)},
			{"Parent", fmt.Sprintf("%s #%d", e.ExpenseableType, e.ExpenseableID)},
			{"Property ID", strconv.Itoa(e.PropertyID)},
			{"Lease ID", derefInt(e.LeaseID)},
		})
		return nil
	},
}

var expensesPaymentsPage int
var expensesPaymentsCmd = &cobra.Command{
	Use:   "payments <expense-id>",
	Short: "List payments for an expense",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if expensesPaymentsPage > 1 {
			params["page"] = strconv.Itoa(expensesPaymentsPage)
		}
		raw, err := client.GetRaw("/expenses/"+args[0]+"/payments", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []payment `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Amount", "Currency", "Period", "Due", "Status", "Approval"}
		rows := make([][]string, len(resp.Data))
		for i, p := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(p.ID),
				fmt.Sprintf("%.2f", p.Amount),
				p.AmountCurrency,
				p.PeriodStart + "→" + p.PeriodEnd,
				p.DueDate,
				p.Status,
				p.ApprovalStatus,
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

var expensesIncreasesCmd = &cobra.Command{
	Use:   "increases <expense-id>",
	Short: "List increases (escalations) for an expense",
	Long:  `Increases are not paginated — the full set is returned.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/expenses/"+args[0]+"/increases", nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []increase `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"Type", "Effective", "Fixed", "Pct", "Applied pct", "Resolved"}
		rows := make([][]string, len(resp.Data))
		for i, x := range resp.Data {
			rows[i] = []string{
				x.IncreaseType,
				x.EffectiveDate,
				derefFloat(x.FixedAmount),
				derefFloat(x.PercentageValue),
				derefFloat(x.AppliedPercentage),
				fmt.Sprintf("%.2f", x.ResolvedAmount),
			}
		}
		printer.Table(headers, rows)
		return nil
	},
}

func init() {
	expensesPaymentsCmd.Flags().IntVar(&expensesPaymentsPage, "page", 1, "Page number")
	expensesCmd.AddCommand(expensesShowCmd)
	expensesCmd.AddCommand(expensesPaymentsCmd)
	expensesCmd.AddCommand(expensesIncreasesCmd)
	rootCmd.AddCommand(expensesCmd)
}
