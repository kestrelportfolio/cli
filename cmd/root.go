// Package cmd contains all CLI commands.
//
// Each file in this package defines one command or command group.
// This is like having one Thor subcommand class per file in a Ruby CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/kestrelportfolio/kestrel-cli/internal/config"
	"github.com/kestrelportfolio/kestrel-cli/internal/output"
	"github.com/spf13/cobra"
)

// These package-level variables are shared across all commands.
// In Ruby terms, these are like module-level instance variables
// that all command classes can access.
var (
	cfg     *config.Config
	client  *api.Client
	printer *output.Printer

	// Flag values
	flagJSON    bool
	flagQuiet   bool
	flagBaseURL string
)

// rootCmd is the base command — what runs when you just type "kestrel".
var rootCmd = &cobra.Command{
	Use:   "kestrel",
	Short: "Kestrel Portfolio CLI",
	Long: `Command-line interface for the Kestrel Portfolio API.
Designed for both human users and AI agents.

Agent integration:
  kestrel commands --json    Discover all available commands
  cat SKILL.md               Read the agent skill documentation`,
	// PersistentPreRunE runs before EVERY subcommand (like a before_action in Rails).
	// It loads config and sets up the shared client/printer.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Apply flag overrides (highest precedence)
		if flagBaseURL != "" {
			cfg.BaseURL = flagBaseURL
		}

		client = api.NewClient(cfg)

		// Set up output mode
		mode := output.ModeAuto
		if flagJSON {
			mode = output.ModeJSON
		} else if flagQuiet {
			mode = output.ModeQuiet
		}
		printer = &output.Printer{Mode: mode}

		return nil
	},
}

// Execute is called from main.go — it's the entry point that kicks off Cobra.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// init runs when the package is loaded (like a Ruby initializer).
// We register global flags here.
func init() {
	// PersistentFlags are inherited by all subcommands (like class_option in Thor).
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output raw JSON")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "Minimal output")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", "API base URL override")
}
