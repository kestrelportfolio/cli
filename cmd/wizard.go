package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/kestrelportfolio/kestrel-cli/internal/config"
	"github.com/kestrelportfolio/kestrel-cli/internal/harness"
	"github.com/spf13/cobra"
)

// runSetupWizard walks a user through auth → agent skill → Claude plugin →
// shell completion in one interactive flow. Intentionally non-scripting;
// refuses to run in --agent or --json mode where explicit subcommands are
// the correct path.
func runSetupWizard() error {
	if printer.IsStructured() {
		return fmt.Errorf("the interactive wizard isn't available in --json/--agent mode. Use the explicit subcommands instead: kestrel login, kestrel skill install, kestrel setup claude, kestrel setup completions")
	}

	fmt.Println()
	fmt.Println("  Kestrel CLI setup")
	fmt.Println("  ─────────────────")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	if err := wizardAuth(reader); err != nil {
		return err
	}
	fmt.Println()
	if err := wizardSkill(reader); err != nil {
		// Skill install is optional — don't fail the whole wizard.
		fmt.Fprintln(os.Stderr, "  Skipping: "+err.Error())
	}
	fmt.Println()
	if err := wizardClaude(reader); err != nil {
		fmt.Fprintln(os.Stderr, "  Skipping: "+err.Error())
	}
	fmt.Println()
	if err := wizardCompletions(reader); err != nil {
		fmt.Fprintln(os.Stderr, "  Skipping: "+err.Error())
	}
	fmt.Println()
	fmt.Println("  ✓ Setup complete.")
	fmt.Println()
	fmt.Println("  Next:")
	fmt.Println("    kestrel me                  # confirm auth")
	fmt.Println("    kestrel properties list     # first real call")
	fmt.Println("    kestrel doctor              # full health check")
	fmt.Println()
	return nil
}

// wizardSkill writes the canonical SKILL.md baseline at ~/.agents/skills/kestrel/
// and offers per-agent fan-out for Codex and OpenCode when they're detected.
// Claude is handled by the next step (wizardClaude) since it has its own
// plugin install path.
//
// The baseline is the source-of-truth for the upgrade-time auto-refresh, so
// it gets installed even if the user declines every per-agent fan-out.
func wizardSkill(reader *bufio.Reader) error {
	fmt.Println("Step 2: Agent skill")

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}
	baselineFile := filepath.Join(home, ".agents", "skills", "kestrel", skillFilename)

	if _, err := os.Stat(baselineFile); err == nil {
		fmt.Printf("  ✓ Baseline skill already installed at %s\n", baselineFile)
	} else {
		if !promptYesNo(reader, "  Install the Kestrel agent skill to ~/.agents/skills/kestrel/?", true) {
			fmt.Println("  Skipped. Install later with: kestrel skill install")
			return nil
		}
		skillPath, err := installSkillFiles()
		if err != nil {
			return fmt.Errorf("installing baseline skill: %w", err)
		}
		fmt.Printf("  ✓ Installed → %s\n", skillPath)
	}

	// Per-agent fan-out. Each entry is a detected non-Claude agent that reads
	// SKILL.md from a fixed location. Claude is intentionally absent — its
	// plugin install in the next step handles its skill registration.
	fanouts := []struct {
		agentID    string
		agentName  string
		targetPath string
	}{
		{"codex", "Codex", codexGlobalSkillPath()},
		{"opencode", "OpenCode", "~/.config/opencode/skill/kestrel/SKILL.md"},
	}

	for _, fanout := range fanouts {
		agent := harness.FindAgent(fanout.agentID)
		if agent == nil || agent.Detect == nil || !agent.Detect() {
			continue
		}

		targetExpanded := expandSkillPath(fanout.targetPath)
		if _, err := os.Stat(targetExpanded); err == nil {
			fmt.Printf("  ✓ %s skill already installed\n", fanout.agentName)
			continue
		}

		if !promptYesNo(reader, fmt.Sprintf("  Also install for %s at %s?", fanout.agentName, fanout.targetPath), true) {
			continue
		}

		if _, _, err := installSkillAt(targetExpanded); err != nil {
			// Per-agent fan-out failure isn't fatal — skip and continue.
			fmt.Fprintln(os.Stderr, "  notice: "+fanout.agentName+" install failed: "+err.Error())
			continue
		}
		fmt.Printf("  ✓ Installed for %s → %s\n", fanout.agentName, targetExpanded)
	}

	return nil
}

// wizardAuth checks whether the configured token still works, and if not,
// prompts for a new one and saves it.
func wizardAuth(reader *bufio.Reader) error {
	fmt.Println("Step 1: Authentication")

	if cfg.Token != "" {
		if me, err := whoAmI(cfg.Token); err == nil {
			fmt.Printf("  ✓ Logged in as %s (%s)\n", me.Email, me.Organization)
			if !promptYesNo(reader, "  Re-authenticate with a different token?", false) {
				return nil
			}
		} else {
			fmt.Printf("  Existing token didn't validate (%s) — let's refresh it.\n", err.Error())
		}
	} else {
		fmt.Println("  No token on file.")
	}

	fmt.Println()
	fmt.Println("  Generate a token at: <web-app>/profile → Preferences → API Access")
	fmt.Print("  Paste your API token: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading token: %w", err)
	}
	token := strings.TrimSpace(line)
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	me, err := whoAmI(token)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if err := config.SaveGlobal(&config.Config{Token: token, BaseURL: cfg.BaseURL}); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	cfg.Token = token
	client.Token = token
	fmt.Printf("  ✓ Logged in as %s (%s)\n", me.Email, me.Organization)
	return nil
}

type wizardMe struct {
	Email        string
	Organization string
}

// whoAmI validates a token by calling /me and extracts the user + org name.
func whoAmI(token string) (*wizardMe, error) {
	testClient := &api.Client{
		BaseURL:    cfg.BaseURL,
		Token:      token,
		HTTPClient: client.HTTPClient,
	}
	env, err := testClient.Get("/me", nil)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 401 {
			return nil, fmt.Errorf("unauthorized")
		}
		return nil, err
	}
	var payload struct {
		User         struct{ Email string } `json:"user"`
		Organization struct{ Name string }  `json:"organization"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		return nil, fmt.Errorf("decoding /me response: %w", err)
	}
	return &wizardMe{Email: payload.User.Email, Organization: payload.Organization.Name}, nil
}

// wizardClaude offers to install the Claude Code plugin when the claude
// binary is on PATH. Silent skip when it isn't — we don't assume every user
// is a Claude Code user.
//
// After the plugin install (or if the plugin was already installed), creates
// the global skill symlink at ~/.claude/skills/kestrel as a fallback for
// users who later remove the plugin — the skill stays registered through the
// baseline. Always best-effort; failures don't fail the wizard.
func wizardClaude(reader *bufio.Reader) error {
	fmt.Println("Step 3: Claude Code integration")

	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Println("  Claude Code binary not on PATH — skipping plugin install.")
		fmt.Println("  Install Claude Code: https://claude.ai/code")
		return nil
	}
	fmt.Println("  ✓ Claude Code detected.")

	if claudePluginInstalled() {
		fmt.Println("  ✓ Kestrel plugin already installed.")
	} else {
		if !promptYesNo(reader, "  Install the Kestrel plugin for Claude Code?", true) {
			fmt.Println("  Skipped. Install later with: kestrel setup claude")
			return nil
		}

		steps, err := harness.RunClaudeSetup()
		for _, step := range steps {
			fmt.Println("  • " + step)
		}
		if err != nil {
			return err
		}
		fmt.Println("  ✓ Plugin installed. Start a new Claude Code session to pick it up.")
	}

	// Tail action: keep the global skill symlink in sync. Handles the
	// "plugin ok, link broken" repair case and gives non-plugin users a
	// fallback registration path.
	if symlinkPath, notice, err := linkSkillToClaude(); err == nil {
		fmt.Printf("  ✓ Linked global skill → %s\n", symlinkPath)
		if notice != "" {
			fmt.Fprintln(os.Stderr, "  notice: "+notice)
		}
	}

	return nil
}

// claudePluginInstalled returns true if the kestrel plugin already appears
// in Claude's plugin list. Mirrors the check in harness/claude.go but is
// local here so the wizard stays self-contained.
func claudePluginInstalled() bool {
	out, err := exec.Command("claude", "plugin", "list", "--json").Output()
	if err != nil {
		return false
	}
	var plugins []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(out, &plugins) != nil {
		return false
	}
	for _, p := range plugins {
		if p.ID == harness.ClaudePluginKey {
			return true
		}
	}
	return false
}

// wizardCompletions appends a `source <(kestrel completion <shell>)` line to
// the user's rc file, after a dedupe check. No-op on unknown shells.
func wizardCompletions(reader *bufio.Reader) error {
	fmt.Println("Step 4: Shell completions")

	plan, err := completionPlanForShell(os.Getenv("SHELL"))
	if err != nil {
		fmt.Println("  " + err.Error())
		return nil
	}
	fmt.Printf("  Detected %s → %s\n", plan.shell, plan.rcPath)

	already, err := rcFileContains(plan.rcPath, plan.marker)
	if err != nil {
		return err
	}
	if already {
		fmt.Println("  ✓ Completions already installed.")
		return nil
	}
	if !promptYesNo(reader, fmt.Sprintf("  Append completions to %s?", plan.rcPath), true) {
		fmt.Println("  Skipped. Install manually later with: kestrel setup completions")
		return nil
	}

	if err := appendCompletionSnippet(plan); err != nil {
		return err
	}
	fmt.Printf("  ✓ Added. Open a new %s shell (or run: source %s) to activate.\n", plan.shell, plan.rcPath)
	return nil
}

// completionPlan holds the rc file + snippet to append for a given shell.
type completionPlan struct {
	shell   string
	rcPath  string
	marker  string // unique comment line used for dedupe
	snippet string
}

// completionPlanForShell maps $SHELL basename to the completion command to
// append. Returns an error for unknown shells so the wizard can skip
// gracefully rather than dump garbage into an rc file.
func completionPlanForShell(shellEnv string) (*completionPlan, error) {
	shell := filepath.Base(shellEnv)
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home dir: %w", err)
	}
	const marker = "# Kestrel CLI completions"
	switch shell {
	case "bash":
		return &completionPlan{
			shell:   "bash",
			rcPath:  filepath.Join(home, ".bashrc"),
			marker:  marker,
			snippet: marker + "\nsource <(kestrel completion bash)\n",
		}, nil
	case "zsh":
		// compdef after the source so zsh wires the completion to the `kestrel`
		// command without requiring the file to live on fpath.
		return &completionPlan{
			shell:   "zsh",
			rcPath:  filepath.Join(home, ".zshrc"),
			marker:  marker,
			snippet: marker + "\nsource <(kestrel completion zsh)\ncompdef _kestrel kestrel\n",
		}, nil
	case "fish":
		return &completionPlan{
			shell:   "fish",
			rcPath:  filepath.Join(home, ".config", "fish", "config.fish"),
			marker:  marker,
			snippet: marker + "\nkestrel completion fish | source\n",
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized shell (%q) — skipping. Run manually: kestrel completion [bash|zsh|fish]", shellEnv)
	}
}

// rcFileContains reports whether the file at path already has our marker.
// Missing file is not an error — we'll create it.
func rcFileContains(path, marker string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	return strings.Contains(string(b), marker), nil
}

// appendCompletionSnippet ensures the rc file's directory exists, then
// appends the snippet. Creates the file if needed.
func appendCompletionSnippet(plan *completionPlan) error {
	if err := os.MkdirAll(filepath.Dir(plan.rcPath), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(plan.rcPath), err)
	}
	f, err := os.OpenFile(plan.rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", plan.rcPath, err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + plan.snippet); err != nil {
		return fmt.Errorf("writing to %s: %w", plan.rcPath, err)
	}
	return nil
}

// promptYesNo reads a single y/n answer with a default. An empty answer
// (just Enter) returns the default.
func promptYesNo(reader *bufio.Reader, question string, defaultYes bool) bool {
	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}
	fmt.Print(question + suffix)
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultYes
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans == "" {
		return defaultYes
	}
	return ans == "y" || ans == "yes"
}

// setupCompletionsCmd is the standalone version of the wizard's completion
// step, for users who already have auth + plugin sorted and just want to
// wire up shell completion.
var setupCompletionsCmd = &cobra.Command{
	Use:   "completions",
	Short: "Install shell completions for the current shell",
	Long: `Appends a completion source line to your shell's rc file (bash, zsh, or
fish). Dedupes via a marker comment, so re-running is safe.

The snippet sources 'kestrel completion <shell>' at shell start, so completions
stay current with whichever kestrel binary is on PATH — no need to regenerate
after upgrades.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if printer.IsStructured() {
			return fmt.Errorf("this command is interactive. Install completions manually: source <(kestrel completion <shell>)")
		}
		reader := bufio.NewReader(os.Stdin)
		return wizardCompletions(reader)
	},
}
