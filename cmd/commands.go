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
