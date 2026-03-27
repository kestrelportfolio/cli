package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/kestrelportfolio/kestrel-cli/internal/harness"
	"github.com/spf13/cobra"
)

// DoctorResult is the JSON output for `kestrel doctor --json`.
type DoctorResult struct {
	Checks []harness.StatusCheck `json:"checks"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check CLI health and agent integrations",
	Long:  `Runs health checks on the CLI, API connectivity, and any detected AI agent integrations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var checks []harness.StatusCheck

		// Check 1: CLI version
		checks = append(checks, harness.StatusCheck{
			Name:   "CLI version",
			Status: "pass",
			Hint:   "kestrel " + api.Version,
		})

		// Check 2: Config file exists with token
		if cfg.Token != "" {
			checks = append(checks, harness.StatusCheck{
				Name:   "API token configured",
				Status: "pass",
			})
		} else {
			checks = append(checks, harness.StatusCheck{
				Name:   "API token configured",
				Status: "fail",
				Hint:   "Run: kestrel login",
			})
		}

		// Check 3: API connectivity (only if we have a token)
		if cfg.Token != "" {
			_, err := client.Get("/me", nil)
			if err != nil {
				apiCheck := harness.StatusCheck{
					Name:   "API connectivity",
					Status: "fail",
					Hint:   err.Error(),
				}
				if apiErr, ok := err.(*api.APIError); ok {
					switch apiErr.Code {
					case "token_expired":
						apiCheck.Hint = "Token expired. Run: kestrel login"
					case "api_disabled":
						apiCheck.Hint = "API access is not enabled for your organization. Contact your admin."
					default:
						apiCheck.Hint = apiErr.Message
					}
				}
				checks = append(checks, apiCheck)
			} else {
				checks = append(checks, harness.StatusCheck{
					Name:   "API connectivity",
					Status: "pass",
				})
			}
		} else {
			checks = append(checks, harness.StatusCheck{
				Name:   "API connectivity",
				Status: "skip",
				Hint:   "No token configured",
			})
		}

		// Check 4+: Agent-specific checks for any detected agents
		for _, agent := range harness.DetectedAgents() {
			if agent.Checks != nil {
				checks = append(checks, agent.Checks()...)
			}
		}

		// Also report agents that aren't detected (informational)
		for _, agent := range harness.AllAgents() {
			if agent.Detect != nil && !agent.Detect() {
				checks = append(checks, harness.StatusCheck{
					Name:   agent.Name + " detected",
					Status: "skip",
					Hint:   agent.Name + " not found on PATH",
				})
			}
		}

		// Output
		if printer.IsJSON() {
			result := DoctorResult{Checks: checks}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		// Styled output
		hasFailure := false
		for _, check := range checks {
			var icon string
			switch check.Status {
			case "pass":
				icon = "✓"
			case "warn":
				icon = "!"
			case "fail":
				icon = "✗"
				hasFailure = true
			case "skip":
				icon = "−"
			}

			line := fmt.Sprintf("%s %s", icon, check.Name)
			if check.Hint != "" {
				if check.Status == "pass" {
					line += fmt.Sprintf(" (%s)", check.Hint)
				} else {
					line += fmt.Sprintf(" — %s", check.Hint)
				}
			}
			fmt.Fprintln(os.Stderr, line)
		}

		if hasFailure {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Some checks failed. See hints above.")
			os.Exit(1)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
