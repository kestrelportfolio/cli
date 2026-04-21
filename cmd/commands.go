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
					Name:        "setup",
					Usage:       "kestrel setup",
					Description: "Interactive wizard: auth → Claude plugin → shell completions",
				},
				{
					Name:        "setup claude",
					Usage:       "kestrel setup claude",
					Description: "Install the Kestrel plugin for Claude Code (non-interactive)",
				},
				{
					Name:        "setup completions",
					Usage:       "kestrel setup completions",
					Description: "Append a completion source line to your shell's rc file",
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
			Category: "Document Parsing",
			Commands: []CommandInfo{
				{
					Name:        "documents parse",
					Usage:       "kestrel documents parse <doc-id> [--wait] [--timeout SECS]",
					Description: "Parse status for the doc's latest version; --wait polls until terminal",
					Flags:       []string{"--wait", "--timeout"},
				},
				{
					Name:        "documents pages",
					Usage:       "kestrel documents pages <doc-id> [--version N]",
					Description: "List pages (width/height/rotation) for a parsed version",
					Flags:       []string{"--version"},
				},
				{
					Name:        "documents blocks",
					Usage:       "kestrel documents blocks <doc-id> [--version N] [--page N] [--type T] [--search Q] [--since-order N] [--near ID --window K] [--limit N]",
					Description: "Walk the reading-ordered block graph. Trigram text search, filters, cursor, neighborhood",
					Flags:       []string{"--version", "--page", "--type", "--search", "--since-order", "--near", "--window", "--limit"},
				},
				{
					Name:        "documents block",
					Usage:       "kestrel documents block <block-id>",
					Description: "Fetch a single block — confirm text + bbox before citing",
				},
			},
		},
		{
			Category: "Abstraction Templates",
			Commands: []CommandInfo{
				{
					Name:        "templates list",
					Usage:       "kestrel templates list [--page N]",
					Description: "List available abstraction templates",
					Flags:       []string{"--page"},
				},
				{
					Name:        "templates show",
					Usage:       "kestrel templates show <id>",
					Description: "Show a template with its ordered requirements",
				},
				{
					Name:        "templates schema",
					Usage:       "kestrel templates schema <id>",
					Description: "Preview the authoring schema a template produces",
				},
			},
		},
		{
			Category: "Abstractions",
			Commands: []CommandInfo{
				{
					Name:        "abstractions list",
					Usage:       "kestrel abstractions list [--page N]",
					Description: "List abstractions",
					Flags:       []string{"--page"},
				},
				{
					Name:        "abstractions show",
					Usage:       "kestrel abstractions show <id>",
					Description: "Show a single abstraction with change/doc counts",
				},
				{
					Name:        "abstractions create",
					Usage:       "kestrel abstractions create --template-id N --kind greenfield|brownfield [--target-property-id N] [--target-lease-id N] [--name ...]",
					Description: "Create a new abstraction",
					Flags:       []string{"--template-id", "--kind", "--target-property-id", "--target-lease-id", "--name"},
				},
				{
					Name:        "abstractions update",
					Usage:       "kestrel abstractions update <id> --name ...",
					Description: "Update an abstraction (name only)",
					Flags:       []string{"--name"},
				},
				{
					Name:        "abstractions abandon",
					Usage:       "kestrel abstractions abandon <id>",
					Description: "Abandon an abstraction (irreversible)",
				},
				{
					Name:        "abstractions schema",
					Usage:       "kestrel abstractions schema <id>",
					Description: "Show the authoring schema — what this abstraction should fill in",
				},
				{
					Name:        "abstractions add-doc",
					Usage:       "kestrel abstractions add-doc <abstraction-id> <file> [--name ...] [--wait-parse] [--timeout SECS]",
					Description: "Upload a file and attach it as a source document; --wait-parse blocks until the structured parse is ready",
					Flags:       []string{"--name", "--wait-parse", "--timeout"},
				},
				{
					Name:        "abstractions remove-doc",
					Usage:       "kestrel abstractions remove-doc <abstraction-id> --document-id N",
					Description: "Destroy a source document (cascade-removes join and pending citing changes)",
					Flags:       []string{"--document-id"},
				},
				{
					Name:        "abstractions sources",
					Usage:       "kestrel abstractions sources <abstraction-id> [--page N]",
					Description: "List source documents attached to an abstraction",
					Flags:       []string{"--page"},
				},
			},
		},
		{
			Category: "Abstraction Changes",
			Commands: []CommandInfo{
				{
					Name:        "abstractions changes list",
					Usage:       "kestrel abstractions changes list <abstraction-id> [--page N] [--state S]...",
					Description: "List staged changes. Filter via --state (repeatable). Rows include a source_links_preview",
					Flags:       []string{"--page", "--state"},
				},
				{
					Name:        "abstractions changes show",
					Usage:       "kestrel abstractions changes show <abstraction-id> <change-id>",
					Description: "Show a change with payload, field metadata, and source links",
				},
				{
					Name:        "abstractions changes create",
					Usage:       "kestrel abstractions changes create <abstraction-id> --action ... --target-type ... --payload ... [--cite-block BLOCK[:chars=S-E|:cell=R,C]]",
					Description: "Create or upsert a staged change. --cite-block is the shortcut for block-ref citations",
					Flags:       []string{"--action", "--target-type", "--target-id", "--target-field", "--sub-object-group", "--parent-change-id", "--revised-from-id", "--payload", "--source-links", "--cite-block"},
				},
				{
					Name:        "abstractions changes create-batch",
					Usage:       "kestrel abstractions changes create-batch <abstraction-id> --file @batch.json",
					Description: "Create up to 500 changes atomically. Per-item errors roll back the whole batch",
					Flags:       []string{"--file"},
				},
				{
					Name:        "abstractions changes update",
					Usage:       "kestrel abstractions changes update <abstraction-id> <change-id> [--payload ...] [--source-links ...]",
					Description: "Update a change's payload or source links (API-sourced pending only; rejected is terminal)",
					Flags:       []string{"--payload", "--source-links"},
				},
				{
					Name:        "abstractions changes delete",
					Usage:       "kestrel abstractions changes delete <abstraction-id> <change-id>",
					Description: "Delete a staged change (API-sourced pending only; rejected is terminal)",
				},
			},
		},
		{
			Category: "Global Flags",
			Commands: []CommandInfo{
				{
					Name:        "--json",
					Usage:       "kestrel <command> --json",
					Description: "Full JSON envelope on stdout (includes breadcrumbs, meta)",
				},
				{
					Name:        "--agent",
					Usage:       "kestrel <command> --agent",
					Description: "Agent/script mode — data-only JSON on success, {ok:false,...} on error",
				},
				{
					Name:        "--quiet",
					Usage:       "kestrel <command> --quiet",
					Description: "Minimal output — suppress success lines and breadcrumbs",
				},
				{
					Name:        "--base-url",
					Usage:       "kestrel <command> --base-url URL",
					Description: "Override the API base URL",
				},
				{
					Name:        "--help --agent",
					Usage:       "kestrel <command> --help --agent",
					Description: "Structured command metadata JSON — walkable tree for agent discovery",
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

		if printer.IsStructured() {
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
