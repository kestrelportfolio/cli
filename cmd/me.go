package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Show current user and organization",
	Long:  `Calls GET /me to show who you're authenticated as. Useful for verifying your token works.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Token == "" {
			return fmt.Errorf("not logged in. Run: kestrel login")
		}

		raw, err := client.GetRaw("/me", nil)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}

		// Parse for styled output
		var resp struct {
			Data struct {
				User struct {
					Email             string `json:"email"`
					DateFormat        string `json:"date_format"`
					Currency          string `json:"currency"`
					MeasurementSystem string `json:"measurement_system"`
					TimeZone          string `json:"time_zone"`
				} `json:"user"`
				Organization struct {
					Name                 string   `json:"name"`
					Subdomain            string   `json:"subdomain"`
					DateFormat           string   `json:"date_format"`
					PresentationCurrency string   `json:"presentation_currency"`
					FunctionalCurrencies []string `json:"functional_currencies"`
					MeasurementSystem    string   `json:"measurement_system"`
				} `json:"organization"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		u := resp.Data.User
		o := resp.Data.Organization

		fmt.Println("User")
		printer.Detail([][]string{
			{"Email", u.Email},
			{"Date format", u.DateFormat},
			{"Currency", u.Currency},
			{"Measurement", u.MeasurementSystem},
			{"Time zone", u.TimeZone},
		})

		fmt.Println()
		fmt.Println("Organization")
		printer.Detail([][]string{
			{"Name", o.Name},
			{"Subdomain", o.Subdomain},
			{"Date format", o.DateFormat},
			{"Currency", o.PresentationCurrency},
			{"Measurement", o.MeasurementSystem},
		})

		return nil
	},
}

func init() {
	rootCmd.AddCommand(meCmd)
}
