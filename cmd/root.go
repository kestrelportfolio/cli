// Package cmd contains all CLI commands.
//
// Each file in this package defines one command or command group.
// This is like having one Thor subcommand class per file in a Ruby CLI.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

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
	flagAgent   bool
	flagBaseURL string
)

// Exit code conventions — modeled on the Basecamp CLI. Scripts and agents
// branch on these to distinguish retryable failures from permanent ones.
const (
	ExitOK        = 0
	ExitUsage     = 1 // bad args, missing required flags, validation (422)
	ExitNotFound  = 2
	ExitAuth      = 3 // no token, 401
	ExitForbidden = 4 // 403
	ExitRateLimit = 5 // 429
	ExitNetwork   = 6 // transport errors
	ExitAPI       = 7 // 5xx and other API-side failures
)

// UsageError is returned by a command's RunE when a required input is missing.
// The root error handler renders it with code:"usage" and the named arg in the
// error message, suitable for agent elicitation loops.
type UsageError struct {
	Arg   string // e.g. "template-id", "payload"
	Usage string // e.g. "kestrel abstractions changes create <abs-id> --action ..."
}

func (e *UsageError) Error() string {
	return fmt.Sprintf("<%s> required", e.Arg)
}

// authMissingError is the sentinel returned by requireLogin when no token
// is configured. It maps to ExitAuth and code:"unauthorized" in the handler.
type authMissingError struct{}

func (e *authMissingError) Error() string { return "not logged in. Run: kestrel login" }

// rootCmd is the base command — what runs when you just type "kestrel".
var rootCmd = &cobra.Command{
	Use:   "kestrel",
	Short: "Kestrel Portfolio CLI",
	Long: `Command-line interface for the Kestrel Portfolio API.
Designed for both human users and AI agents.

Agent integration:
  kestrel commands --json    Discover all available commands
  kestrel <cmd> --help       Inline command docs
  cat skills/kestrel/SKILL.md    Full agent skill documentation`,
	SilenceErrors: true, // we render errors ourselves (see Execute)
	SilenceUsage:  true,
	// PersistentPreRunE runs before EVERY subcommand (like a before_action in Rails).
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if flagBaseURL != "" {
			cfg.BaseURL = flagBaseURL
		}

		client = api.NewClient(cfg)

		mode := output.ModeAuto
		switch {
		case flagAgent:
			mode = output.ModeAgent
		case flagJSON:
			mode = output.ModeJSON
		case flagQuiet:
			mode = output.ModeQuiet
		}
		printer = &output.Printer{Mode: mode}

		return nil
	},
}

// Execute is called from main.go — it runs the command tree and handles
// errors uniformly: structured JSON to stdout in --json/--agent mode,
// "Error: …" to stderr otherwise, and a categorized exit code.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		os.Exit(ExitOK)
	}
	renderError(err)
	os.Exit(exitCodeFor(err))
}

// renderError emits the error to the right stream in the right shape.
// Falls back to os.Args inspection when printer isn't set yet (e.g. cobra
// arg-count errors that fire before PersistentPreRunE runs).
func renderError(err error) {
	structured := (printer != nil && printer.IsStructured()) || argsWantStructured()
	if structured {
		renderStructuredError(err)
		return
	}
	fmt.Fprintln(os.Stderr, "Error: "+err.Error())
}

func argsWantStructured() bool {
	for _, a := range os.Args {
		if a == "--agent" || a == "--json" {
			return true
		}
	}
	return false
}

func renderStructuredError(err error) {
	env := map[string]any{"ok": false, "error": err.Error()}

	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		env["code"] = "usage"
		if usageErr.Usage != "" {
			env["hint"] = usageErr.Usage
		}
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code != "" {
			env["code"] = apiErr.Code
		}
		if len(apiErr.Errors) > 0 {
			env["errors"] = apiErr.Errors
			if _, has := env["code"]; !has {
				env["code"] = "validation"
			}
		}
	}

	var authErr *authMissingError
	if errors.As(err, &authErr) {
		env["code"] = "unauthorized"
		env["hint"] = "kestrel login"
	}

	// Fallback: uncoded errors are usually CLI-input problems (cobra arg-parse
	// errors, unknown flags, etc.). Label them "usage" so agents can branch.
	if _, hasCode := env["code"]; !hasCode {
		env["code"] = "usage"
	}

	out, encErr := json.MarshalIndent(env, "", "  ")
	if encErr != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		return
	}
	fmt.Println(string(out))
}

// exitCodeFor maps an error to one of the documented exit codes.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return ExitUsage
	}
	var authErr *authMissingError
	if errors.As(err, &authErr) {
		return ExitAuth
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return apiErrorExit(apiErr)
	}
	// Transport-level network errors.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return ExitNetwork
	}
	// Message-level sniff for "no such host", "connection refused", etc.
	if msg := err.Error(); strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "dial tcp") {
		return ExitNetwork
	}
	return ExitUsage
}

func apiErrorExit(e *api.APIError) int {
	switch e.Code {
	case "unauthorized", "token_expired":
		return ExitAuth
	case "api_disabled", "forbidden":
		return ExitForbidden
	case "not_found":
		return ExitNotFound
	case "rate_limited":
		return ExitRateLimit
	}
	switch e.StatusCode {
	case 401:
		return ExitAuth
	case 403:
		return ExitForbidden
	case 404:
		return ExitNotFound
	case 422:
		return ExitUsage
	case 429:
		return ExitRateLimit
	}
	if e.StatusCode >= 500 {
		return ExitAPI
	}
	if len(e.Errors) > 0 {
		return ExitUsage
	}
	return ExitAPI
}

// init runs when the package is loaded (like a Ruby initializer).
// We register global flags here.
func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output the full JSON envelope (auto when piped)")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "Minimal output — suppress stderr hints and success lines")
	rootCmd.PersistentFlags().BoolVar(&flagAgent, "agent", false, "Data-only JSON on success; {ok:false,...} on error. For scripts and AI agents.")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", "API base URL override")
}
