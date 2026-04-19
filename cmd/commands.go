package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"os"

	"github.com/spf13/cobra"
)

// CommandInfo describes a single command for agent discovery.
// These are hand-maintained rather than introspected from Cobra —
// this lets us control exactly what agents see, like a curated menu
// vs. dumping every internal method.
type CommandInfo struct {
	Name        string   `json:"name"`
	Usage       string   `json:"usage"`
	Description string   `json:"description"`
	Flags       []string `json:"flags,omitempty"`
}

// CommandCategory groups related commands.
type CommandCategory struct {
	Category string        `json:"category"`
	Commands []CommandInfo `json:"commands"`
}

// commandCatalog is the static registry of all commands.
// Update this whenever you add a new command.
func commandCatalog() []CommandCategory {
	return []CommandCategory{
		{
			Category: "Core Commands",
			Commands: []CommandInfo{
				{
					Name:        "login",
					Usage:       "kestrel login [--token TOKEN]",
					Description: "Authenticate with the Kestrel Portfolio API",
					Flags:       []string{"--token"},
				},
				{
					Name:        "me",
					Usage:       "kestrel me",
					Description: "Show current user and organization",
				},
				{
					Name:        "commands",
					Usage:       "kestrel commands",
					Description: "List all available commands",
				},
				{
					Name:        "doctor",
					Usage:       "kestrel doctor",
					Description: "Check CLI health and agent integrations",
				},
				{
					Name:        "version",
					Usage:       "kestrel version",
					Description: "Print the CLI version",
				},
			},
		},
		{
			Category: "Agent Setup",
			Commands: []CommandInfo{
				{
					Name:        "setup claude",
					Usage:       "kestrel setup claude",
					Description: "Install the Kestrel plugin for Claude Code",
				},
			},
		},
		{
			Category: "Configuration",
			Commands: []CommandInfo{
				{
					Name:        "field-configs list",
					Usage:       "kestrel field-configs list",
					Description: "Org field configuration (captions, required, options) per model",
				},
			},
		},
		{
			Category: "Properties",
			Commands: []CommandInfo{
				{
					Name:        "properties list",
					Usage:       "kestrel properties list [--page N]",
					Description: "List properties (paginated, 50 per page)",
					Flags:       []string{"--page"},
				},
				{
					Name:        "properties show",
					Usage:       "kestrel properties show <id>",
					Description: "Show a single property by ID",
				},
				{
					Name:        "properties leases",
					Usage:       "kestrel properties leases <property-id> [--page N]",
					Description: "List leases on a property",
					Flags:       []string{"--page"},
				},
				{
					Name:        "properties expenses",
					Usage:       "kestrel properties expenses <property-id> [--page N]",
					Description: "List property-level expenses",
					Flags:       []string{"--page"},
				},
				{
					Name:        "properties documents",
					Usage:       "kestrel properties documents <property-id> [--page N]",
					Description: "List documents linked to a property",
					Flags:       []string{"--page"},
				},
				{
					Name:        "properties key-dates",
					Usage:       "kestrel properties key-dates <property-id> [--page N]",
					Description: "List property-level key dates",
					Flags:       []string{"--page"},
				},
				{
					Name:        "properties date-entries",
					Usage:       "kestrel properties date-entries <property-id>",
					Description: "Full date dependency graph for a property (not paginated)",
				},
			},
		},
		{
			Category: "Leases",
			Commands: []CommandInfo{
				{
					Name:        "leases list",
					Usage:       "kestrel leases list [--page N]",
					Description: "List leases (paginated, 50 per page)",
					Flags:       []string{"--page"},
				},
				{
					Name:        "leases show",
					Usage:       "kestrel leases show <id>",
					Description: "Show a single lease by ID",
				},
				{
					Name:        "leases expenses",
					Usage:       "kestrel leases expenses <lease-id> [--page N]",
					Description: "List expenses on a lease",
					Flags:       []string{"--page"},
				},
				{
					Name:        "leases documents",
					Usage:       "kestrel leases documents <lease-id> [--page N]",
					Description: "List documents linked to a lease",
					Flags:       []string{"--page"},
				},
				{
					Name:        "leases key-dates",
					Usage:       "kestrel leases key-dates <lease-id> [--page N]",
					Description: "List lease-level key dates",
					Flags:       []string{"--page"},
				},
				{
					Name:        "leases component-areas",
					Usage:       "kestrel leases component-areas <lease-id> [--page N]",
					Description: "List component areas for a lease",
					Flags:       []string{"--page"},
				},
				{
					Name:        "leases securities",
					Usage:       "kestrel leases securities <lease-id> [--page N]",
					Description: "List lease securities (deposits, guarantees, LOCs)",
					Flags:       []string{"--page"},
				},
				{
					Name:        "leases clauses",
					Usage:       "kestrel leases clauses <lease-id> [--page N]",
					Description: "List lease clauses",
					Flags:       []string{"--page"},
				},
			},
		},
		{
			Category: "Expenses",
			Commands: []CommandInfo{
				{
					Name:        "expenses show",
					Usage:       "kestrel expenses show <id>",
					Description: "Show a single expense",
				},
				{
					Name:        "expenses payments",
					Usage:       "kestrel expenses payments <expense-id> [--page N]",
					Description: "List payments for an expense",
					Flags:       []string{"--page"},
				},
				{
					Name:        "expenses increases",
					Usage:       "kestrel expenses increases <expense-id>",
					Description: "List increases (escalations) for an expense",
				},
			},
		},
		{
			Category: "Lease Securities",
			Commands: []CommandInfo{
				{
					Name:        "lease-securities show",
					Usage:       "kestrel lease-securities show <id>",
					Description: "Show a single lease security",
				},
				{
					Name:        "lease-securities increases",
					Usage:       "kestrel lease-securities increases <security-id>",
					Description: "List increases for a lease security",
				},
			},
		},
		{
			Category: "Documents",
			Commands: []CommandInfo{
				{
					Name:        "documents show",
					Usage:       "kestrel documents show <id>",
					Description: "Show document metadata",
				},
				{
					Name:        "documents download",
					Usage:       "kestrel documents download <id> [--version N] [-o FILE] [--url]",
					Description: "Download a document's file or print its signed URL",
					Flags:       []string{"--version", "-o", "--url"},
				},
			},
		},
		{
			Category: "Global Flags",
			Commands: []CommandInfo{
				{
					Name:        "--json",
					Usage:       "kestrel <command> --json",
					Description: "Output raw JSON (default when piped)",
				},
				{
					Name:        "--quiet",
					Usage:       "kestrel <command> --quiet",
					Description: "Minimal output for scripting",
				},
				{
					Name:        "--base-url",
					Usage:       "kestrel <command> --base-url URL",
					Description: "Override the API base URL",
				},
			},
		},
	}
}

var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List all available commands",
	Long:  `Lists all available commands with descriptions and flags. Use --json for agent-readable output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		catalog := commandCatalog()

		if printer.IsJSON() {
			data, err := json.MarshalIndent(catalog, "", "  ")
			if err != nil {
				return fmt.Errorf("encoding commands: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}

		// Styled TTY output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for i, cat := range catalog {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, cat.Category)
			fmt.Fprintln(w, strings.Repeat("─", len(cat.Category)))
			for _, c := range cat.Commands {
				fmt.Fprintf(w, "  %s\t%s\n", c.Usage, c.Description)
			}
		}
		w.Flush()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(commandsCmd)
}
