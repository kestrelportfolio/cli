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

var leasesPage int

var leasesCmd = &cobra.Command{
	Use:   "leases",
	Short: "Manage leases",
}

var leasesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List leases",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Token == "" {
			return fmt.Errorf("not logged in. Run: kestrel login")
		}

		params := map[string]string{}
		if leasesPage > 1 {
			params["page"] = strconv.Itoa(leasesPage)
		}

		raw, err := client.GetRaw("/leases", params)
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

		return nil
	},
}

var leasesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a lease",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Token == "" {
			return fmt.Errorf("not logged in. Run: kestrel login")
		}

		raw, err := client.GetRaw("/leases/"+args[0], nil)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}

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

		return nil
	},
}

func init() {
	leasesListCmd.Flags().IntVar(&leasesPage, "page", 1, "Page number")
	leasesCmd.AddCommand(leasesListCmd)
	leasesCmd.AddCommand(leasesShowCmd)
	rootCmd.AddCommand(leasesCmd)
}
