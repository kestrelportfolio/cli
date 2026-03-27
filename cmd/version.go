package cmd

import (
	"fmt"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kestrel %s\n", api.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
