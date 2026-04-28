package cmd

import (
	"fmt"
	"os"

	"github.com/kestrelportfolio/kestrel-cli/internal/config"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the saved API token",
	Long:  `Clears the API token from ~/.config/kestrel/config.json. Does not affect KESTREL_TOKEN env var or local .kestrel/config.json.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cleared, err := config.ClearToken()
		if err != nil {
			return fmt.Errorf("clearing token: %w", err)
		}

		configPath, _ := config.GlobalConfigPath()

		if !cleared {
			if printer.IsStructured() {
				printer.Success("No token was set.")
				return nil
			}
			printer.Success("No token was set — nothing to clear.")
			return nil
		}

		if printer.IsStructured() {
			printer.Success("Logged out.")
			return nil
		}

		printer.Success("Logged out. Token removed.")
		fmt.Fprintf(os.Stderr, "  Updated %s\n", configPath)

		if os.Getenv("KESTREL_TOKEN") != "" {
			fmt.Fprintln(os.Stderr, "  Note: KESTREL_TOKEN env var is still set and will continue to authenticate requests.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
