package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

type leaseSecurity struct {
	ID             int      `json:"id"`
	LeaseID        int      `json:"lease_id"`
	SecurityType   string   `json:"security_type"`
	Status         *string  `json:"status"`
	Amount         *float64 `json:"amount"`
	CurrentAmount  *float64 `json:"current_amount"`
	AmountCurrency *string  `json:"amount_currency"`
	RequiredDate   *string  `json:"required_date"`
	ReturnDate     *string  `json:"return_date"`
}

var leaseSecuritiesCmd = &cobra.Command{
	Use:     "lease-securities",
	Aliases: []string{"lease_securities"},
	Short:   "Inspect lease securities (deposits, guarantees, LOCs) and their increases",
}

var leaseSecuritiesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a single lease security",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/lease_securities/"+args[0], nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data leaseSecurity `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		s := resp.Data
		printer.Detail([][]string{
			{"ID", strconv.Itoa(s.ID)},
			{"Lease ID", strconv.Itoa(s.LeaseID)},
			{"Type", s.SecurityType},
			{"Status", deref(s.Status)},
			{"Amount", derefFloat(s.Amount)},
			{"Current amount", derefFloat(s.CurrentAmount)},
			{"Currency", deref(s.AmountCurrency)},
			{"Required date", deref(s.RequiredDate)},
			{"Return date", deref(s.ReturnDate)},
		})
		return nil
	},
}

var leaseSecuritiesIncreasesCmd = &cobra.Command{
	Use:   "increases <security-id>",
	Short: "List increases for a lease security",
	Long:  `Increases are not paginated — the full set is returned.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/lease_securities/"+args[0]+"/increases", nil)
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
	leaseSecuritiesCmd.AddCommand(leaseSecuritiesShowCmd)
	leaseSecuritiesCmd.AddCommand(leaseSecuritiesIncreasesCmd)
	rootCmd.AddCommand(leaseSecuritiesCmd)
}
