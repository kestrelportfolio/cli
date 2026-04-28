package harness

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// Claude Code marketplace and plugin identifiers.
const (
	ClaudeMarketplaceSource = "kestrelportfolio/claude-plugins"
	ClaudePluginKey         = "kestrel@kestrel-plugins"
)

// init registers Claude Code as an agent.
// Go's init() runs automatically when the package is imported —
// similar to a Ruby initializer or a JS module's top-level code.
func init() {
	RegisterAgent(AgentInfo{
		Name:   "Claude Code",
		ID:     "claude",
		Detect: detectClaude,
		Checks: checkClaude,
	})
}

// detectClaude returns true if the `claude` binary is on PATH.
func detectClaude() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// DetectClaude is the exported form of detectClaude — used by callers outside
// this package (e.g. the skill installer) that need to branch on Claude
// being present without going through the registry.
func DetectClaude() bool {
	return detectClaude()
}

// checkClaude runs health checks for Claude Code integration.
func checkClaude() []StatusCheck {
	var checks []StatusCheck

	// Check 1: Is claude binary available?
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		checks = append(checks, StatusCheck{
			Name:   "Claude Code installed",
			Status: "fail",
			Hint:   "Install Claude Code: https://claude.ai/code",
		})
		return checks // Can't check further without the binary
	}
	checks = append(checks, StatusCheck{
		Name:   "Claude Code installed",
		Status: "pass",
	})

	// Check 2: Is the kestrel plugin installed?
	// Run `claude plugin list --json` and look for our specific plugin ID
	pluginInstalled := false
	out, err := exec.Command(claudePath, "plugin", "list", "--json").Output()
	if err == nil {
		// Parse the JSON array and look for our exact plugin key
		var plugins []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(out, &plugins) == nil {
			for _, p := range plugins {
				if p.ID == ClaudePluginKey {
					pluginInstalled = true
					break
				}
			}
		}
	}

	if pluginInstalled {
		checks = append(checks, StatusCheck{
			Name:   "Kestrel plugin installed",
			Status: "pass",
		})
	} else {
		checks = append(checks, StatusCheck{
			Name:   "Kestrel plugin installed",
			Status: "fail",
			Hint:   "Run: kestrel setup claude",
		})
	}

	return checks
}

// RunClaudeSetup performs the Claude Code plugin installation.
// Returns a list of steps taken and any error encountered.
func RunClaudeSetup() ([]string, error) {
	var steps []string

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return steps, &SetupError{
			Message: "Claude Code not found on PATH",
			Hint:    "Install Claude Code first: https://claude.ai/code",
		}
	}

	// Step 1: Add the marketplace
	cmd := exec.Command(claudePath, "plugin", "marketplace", "add", ClaudeMarketplaceSource)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Marketplace may already be added — that's OK
		if !strings.Contains(string(out), "already") {
			return steps, &SetupError{
				Message: "Failed to add marketplace: " + string(out),
				Hint:    "Try manually: claude plugin marketplace add " + ClaudeMarketplaceSource,
			}
		}
	}
	steps = append(steps, "Marketplace added")

	// Step 2: Install the plugin
	cmd = exec.Command(claudePath, "plugin", "install", ClaudePluginKey)
	out, err = cmd.CombinedOutput()
	if err != nil {
		if !strings.Contains(string(out), "already") {
			return steps, &SetupError{
				Message: "Failed to install plugin: " + string(out),
				Hint:    "Try manually: claude plugin install " + ClaudePluginKey,
			}
		}
	}
	steps = append(steps, "Plugin installed")

	return steps, nil
}

// ClaudeSetupResult is the JSON output for `kestrel setup claude --json`.
type ClaudeSetupResult struct {
	Agent     string   `json:"agent"`
	Status    string   `json:"status"` // "success" or "error"
	Steps     []string `json:"steps"`
	Error     string   `json:"error,omitempty"`
	NextSteps string   `json:"next_steps,omitempty"`
}

// ToJSON serializes the result for --json output.
func (r *ClaudeSetupResult) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// SetupError is returned when agent setup fails.
type SetupError struct {
	Message string
	Hint    string
}

func (e *SetupError) Error() string {
	return e.Message
}
