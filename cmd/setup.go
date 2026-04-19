package cmd

import (
	"fmt"
	"os"

	"github.com/kestrelportfolio/kestrel-cli/internal/harness"
	"github.com/spf13/cobra"
)

// setupCmd is the parent command: `kestrel setup`.
// Subcommands are generated dynamically from the agent registry —
// one per registered agent (e.g., `kestrel setup claude`).
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up AI agent integrations",
	Long:  `Installs the Kestrel plugin for your AI coding agent so it can discover and use CLI commands.`,
}

func init() {
	// Generate a subcommand for each registered agent.
	// This is the Go equivalent of Ruby's method_missing or dynamic method definition —
	// we loop over a registry and create commands at startup.
	for _, agent := range harness.AllAgents() {
		a := agent // capture for closure (Go gotcha: loop vars are reused)
		setupCmd.AddCommand(&cobra.Command{
			Use:   a.ID,
			Short: fmt.Sprintf("Install the Kestrel plugin for %s", a.Name),
			Long:  fmt.Sprintf("Sets up the %s integration so %s can discover and use Kestrel commands.", a.Name, a.Name),
			RunE:  makeSetupHandler(a),
		})
	}
	rootCmd.AddCommand(setupCmd)
}

// makeSetupHandler returns the RunE function for a specific agent's setup command.
// Right now only Claude is registered, but this dispatches generically.
func makeSetupHandler(agent harness.AgentInfo) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		switch agent.ID {
		case "claude":
			return runClaudeSetup()
		default:
			return fmt.Errorf("setup not implemented for %s", agent.Name)
		}
	}
}

func runClaudeSetup() error {
	steps, err := harness.RunClaudeSetup()

	if printer.IsStructured() {
		result := &harness.ClaudeSetupResult{
			Agent: "claude",
			Steps: steps,
		}
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
			if se, ok := err.(*harness.SetupError); ok {
				result.NextSteps = se.Hint
			}
		} else {
			result.Status = "success"
			result.NextSteps = "Start a new Claude Code session to use Kestrel commands."
		}
		data, _ := result.ToJSON()
		fmt.Println(string(data))
		if err != nil {
			os.Exit(1)
		}
		return nil
	}

	// Styled output
	for _, step := range steps {
		printer.Success(step)
	}

	if err != nil {
		if se, ok := err.(*harness.SetupError); ok {
			printer.Errorf("%s", se.Message)
			fmt.Fprintf(os.Stderr, "  Hint: %s\n", se.Hint)
		} else {
			printer.Errorf("%s", err.Error())
		}
		return err
	}

	fmt.Fprintln(os.Stderr)
	printer.Success("Claude Code integration ready")
	fmt.Fprintln(os.Stderr, "  Start a new Claude Code session to use Kestrel commands.")

	return nil
}
