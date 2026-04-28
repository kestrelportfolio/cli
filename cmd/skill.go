package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/kestrelportfolio/kestrel-cli/internal/config"
	"github.com/kestrelportfolio/kestrel-cli/internal/harness"
	"github.com/kestrelportfolio/kestrel-cli/skills"
)

// skillLocation is one entry in the install-target list. The picker shows the
// human name and path; the auto-refresh added later sweeps the same list.
type skillLocation struct {
	Name string
	Path string // may use ~ or be project-relative — expansion happens at write time
}

// skillLocations is the canonical set of install destinations. Keep ordered
// from most-shared (~/.agents/) to most-specific (project-local). Project-
// relative entries are intentionally absolute-prefixed-only at refresh time
// (no reliable project root from a PostRunE hook).
var skillLocations = []skillLocation{
	{Name: "Agents (Shared)", Path: "~/.agents/skills/kestrel/SKILL.md"},
	{Name: "Claude Code (Global)", Path: "~/.claude/skills/kestrel/SKILL.md"},
	{Name: "Claude Code (Project)", Path: ".claude/skills/kestrel/SKILL.md"},
	{Name: "OpenCode (Global)", Path: "~/.config/opencode/skill/kestrel/SKILL.md"},
	{Name: "OpenCode (Project)", Path: ".opencode/skill/kestrel/SKILL.md"},
	{Name: "Codex (Global)", Path: codexGlobalSkillPath()},
}

const (
	// skillEmbeddedPath is the location of SKILL.md inside skills.FS.
	skillEmbeddedPath = "kestrel/SKILL.md"

	// skillFilename is the on-disk name we always use for the installed skill.
	skillFilename = "SKILL.md"

	// installedVersionFile records which CLI version wrote the baseline.
	// Read by the upgrade-time refresh added in a follow-up step.
	installedVersionFile = ".installed-version"
)

// skillCmd is `kestrel skill`. Behavior splits on output mode:
//   - non-interactive (piped, --json, --agent): print the embedded SKILL.md
//     to stdout — useful for piping into a file or eyeballing the contents
//     shipped with this binary.
//   - interactive TTY: run the picker so the user can pick where to install.
//
// Mirrors `basecamp skill`: the same command name does either job depending
// on context. Scripts use 'kestrel skill install' for non-interactive writes.
var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Print or install the embedded agent skill file",
	Long: `Prints the SKILL.md embedded in this binary, or — in an interactive shell —
opens a picker for installing it to one of the known agent locations.

Non-interactive (piped, --json, --agent): prints SKILL.md to stdout.
Interactive TTY: shows a picker over Claude / Codex / OpenCode locations.

For scripted installs, use 'kestrel skill install' (writes to the canonical
baseline at ~/.agents/skills/kestrel/ unconditionally).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if printer.IsStructured() {
			data, err := skills.FS.ReadFile(skillEmbeddedPath)
			if err != nil {
				return fmt.Errorf("reading embedded skill: %w", err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), string(data))
			return err
		}
		return runSkillWizard(bufio.NewReader(os.Stdin))
	},
}

// skillInstallCmd writes the baseline and (when Claude is present) symlinks
// the Claude global skills directory to it. Idempotent — safe to re-run.
var skillInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the Kestrel agent skill to ~/.agents/skills/kestrel/",
	Long: `Writes the embedded SKILL.md to ~/.agents/skills/kestrel/ — the vendor-neutral
baseline that Claude Code, Codex, and OpenCode can all read. When Claude Code
is detected, also creates a symlink at ~/.claude/skills/kestrel pointing back
to the baseline so updates propagate automatically on CLI upgrade.

Idempotent: safe to re-run.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		skillPath, err := installSkillFiles()
		if err != nil {
			return err
		}

		result := map[string]any{"skill_path": skillPath}

		if harness.DetectClaude() {
			symlinkPath, notice, linkErr := linkSkillToClaude()
			if linkErr != nil {
				return linkErr
			}
			result["symlink_path"] = symlinkPath
			if notice != "" {
				// Notice means the symlink failed and we copied instead — surface
				// it on stderr regardless of output mode so the user sees it.
				result["notice"] = notice
				fmt.Fprintln(os.Stderr, "  notice: "+notice)
			}
		}

		// Summary + breadcrumbs feed all three output modes through one path:
		//   TTY   → "✓ summary" / "→ breadcrumb" on stderr
		//   --json → merged into the envelope
		//   --agent → stripped (only `data` is emitted)
		printer.Summary(fmt.Sprintf("Kestrel skill installed → %s", skillPath))
		if sym, ok := result["symlink_path"].(string); ok {
			printer.Breadcrumb("Linked to Claude at " + sym)
		}

		printer.FinishEnvelope(map[string]any{
			"ok":   true,
			"data": result,
		})
		return nil
	},
}

// installSkillFiles writes the embedded SKILL.md to ~/.agents/skills/kestrel/
// along with a .installed-version sentinel. Returns the absolute path of the
// installed SKILL.md. Thin wrapper around installSkillAt for the canonical
// baseline location — used by the non-interactive `skill install` command.
func installSkillFiles() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	baseline := filepath.Join(home, ".agents", "skills", "kestrel", skillFilename)
	skillPath, _, err := installSkillAt(baseline)
	return skillPath, err
}

// installSkillAt writes the embedded SKILL.md to expandedPath and (when that
// path isn't itself the canonical baseline) mirrors a copy to ~/.agents/skills/
// kestrel/. Always stamps the .installed-version sentinel in the baseline dir
// so the upgrade-time refresh has a stable reference.
//
// Returns (skillPath, baselinePath, err). baselinePath is empty when no mirror
// was performed (target IS the baseline) — callers use that to decide whether
// to surface a "mirrored to baseline" message.
func installSkillAt(expandedPath string) (string, string, error) {
	data, err := skills.FS.ReadFile(skillEmbeddedPath)
	if err != nil {
		return "", "", fmt.Errorf("reading embedded skill: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(expandedPath), 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", filepath.Dir(expandedPath), err)
	}
	if err := os.WriteFile(expandedPath, data, 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", expandedPath, err)
	}

	var baselinePath string
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		baselineDir := filepath.Join(home, ".agents", "skills", "kestrel")
		baselineFile := filepath.Join(baselineDir, skillFilename)
		if baselineFile != expandedPath {
			if mkErr := os.MkdirAll(baselineDir, 0o755); mkErr == nil {
				if wErr := os.WriteFile(baselineFile, data, 0o644); wErr == nil {
					baselinePath = baselineFile
				}
			}
		}
		// Always stamp the sentinel — used by the upgrade-time refresh.
		_ = os.WriteFile(filepath.Join(baselineDir, installedVersionFile), []byte(api.Version), 0o644)
	}

	return expandedPath, baselinePath, nil
}

// runSkillOnlySetup is the dispatch handler for `kestrel setup <agent>` when
// the agent's setup is just "drop SKILL.md in its conventional location"
// (Codex, OpenCode — no plugin/marketplace involved). Mirrors Step 1's
// install command in shape, but writes to the per-agent location and
// surfaces the agent name in the summary.
func runSkillOnlySetup(agentName, locationPath string) error {
	skillPath, baselinePath, err := installSkillAt(expandSkillPath(locationPath))
	if err != nil {
		return err
	}

	result := map[string]any{"skill_path": skillPath}
	if baselinePath != "" {
		result["baseline_path"] = baselinePath
	}

	printer.Summary(fmt.Sprintf("%s skill installed → %s", agentName, skillPath))
	if baselinePath != "" {
		printer.Breadcrumb("Mirrored to baseline at " + baselinePath)
	}

	printer.FinishEnvelope(map[string]any{
		"ok":   true,
		"data": result,
	})
	return nil
}

// linkSkillToClaude creates a symlink at ~/.claude/skills/kestrel pointing to
// the baseline at ~/.agents/skills/kestrel. Falls back to a directory copy
// when the OS/filesystem won't allow symlinks (some Windows configurations).
//
// Returns (path, notice, err). A non-empty notice means we used the copy
// fallback — surface it so the user knows this install won't auto-update on
// the next CLI upgrade.
func linkSkillToClaude() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("getting home directory: %w", err)
	}

	baselineDir := filepath.Join(home, ".agents", "skills", "kestrel")
	symlinkDir := filepath.Join(home, ".claude", "skills")
	symlinkPath := filepath.Join(symlinkDir, "kestrel")

	if err := os.MkdirAll(symlinkDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating symlink parent: %w", err)
	}

	// Idempotent: clear whatever is at the path so the new entry is clean.
	// RemoveAll handles both a stale symlink and a previous copy-fallback dir.
	_ = os.RemoveAll(symlinkPath)

	target := filepath.Join("..", "..", ".agents", "skills", "kestrel")
	if err := os.Symlink(target, symlinkPath); err == nil {
		return symlinkPath, "", nil
	}

	if err := copySkillDir(baselineDir, symlinkPath); err != nil {
		return "", "", fmt.Errorf("symlink fallback copy failed: %w", err)
	}
	return symlinkPath, "symlink unavailable; copied skill files instead (won't auto-update on upgrade)", nil
}

// copySkillDir copies all top-level files from src to dst. The skill directory
// is intentionally flat (SKILL.md + version sentinel) so we don't recurse.
func copySkillDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(src, entry.Name()))
		if readErr != nil {
			return readErr
		}
		if writeErr := os.WriteFile(filepath.Join(dst, entry.Name()), b, 0o644); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// runSkillWizard prompts for an install location and writes SKILL.md there
// plus to the canonical ~/.agents/skills/kestrel/ baseline. The dual-write
// matters because the upgrade-time refresh reads from the baseline — without
// it, picking only "Codex (Global)" would leave the canonical empty and
// future refreshes would have nothing to copy from.
func runSkillWizard(reader *bufio.Reader) error {
	fmt.Println()
	fmt.Println("  Kestrel skill installation")
	fmt.Println("  ──────────────────────────")
	fmt.Println()
	fmt.Println("  Where would you like to install the Kestrel skill?")
	fmt.Println()

	for i, loc := range skillLocations {
		fmt.Printf("    %d. %-22s (%s)\n", i+1, loc.Name, loc.Path)
	}
	otherIdx := len(skillLocations) + 1
	fmt.Printf("    %d. Other (custom path)\n", otherIdx)
	fmt.Println()

	choice, ok := promptChoice(reader, otherIdx)
	if !ok {
		fmt.Println("  Installation canceled.")
		return nil
	}

	var selectedPath string
	if choice == otherIdx {
		fmt.Print("  Enter custom path: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("  Installation canceled.")
			return nil
		}
		custom := strings.TrimSpace(line)
		if custom == "" {
			fmt.Println("  Installation canceled.")
			return nil
		}
		selectedPath = normalizeSkillPath(custom)
	} else {
		selectedPath = skillLocations[choice-1].Path
	}

	expandedPath := expandSkillPath(selectedPath)

	if _, err := os.Stat(expandedPath); err == nil {
		if !promptYesNo(reader, fmt.Sprintf("  File already exists at %s. Overwrite?", selectedPath), false) {
			fmt.Println("  Installation canceled.")
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", expandedPath, err)
	}

	skillPath, baselinePath, err := installSkillAt(expandedPath)
	if err != nil {
		return err
	}
	fmt.Printf("  ✓ Installed → %s\n", skillPath)
	if baselinePath != "" {
		fmt.Printf("  ✓ Mirrored to baseline → %s\n", baselinePath)
	}
	return nil
}

// promptChoice reads a 1-based numeric choice from the user. Returns
// (n, true) on a valid pick; (0, false) on blank input, EOF, or invalid
// entry. Cancel-on-bad-input keeps the prompt simple — no retry loop.
func promptChoice(reader *bufio.Reader, max int) (int, bool) {
	fmt.Printf("  Choice [1-%d, blank to cancel]: ", max)
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(line)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > max {
		return 0, false
	}
	return n, true
}

// normalizeSkillPath turns a user-entered path into a full SKILL.md path.
// Rules (mirrors basecamp's helper, scoped to "kestrel"):
//   - any *.md path is taken as-is (user knows what they want)
//   - a path ending in "kestrel" gets SKILL.md appended
//   - bare directories get "kestrel/SKILL.md" appended
//
// Lets users type just "~/.claude/skills" and have it land correctly.
func normalizeSkillPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasSuffix(strings.ToLower(path), ".md") {
		return path
	}
	if strings.HasSuffix(path, "kestrel") || strings.HasSuffix(path, "kestrel/") || strings.HasSuffix(path, "kestrel\\") {
		return filepath.Join(path, skillFilename)
	}
	return filepath.Join(path, "kestrel", skillFilename)
}

// expandSkillPath expands a leading ~/ to the user's home directory.
// Project-relative paths (no ~ prefix, no leading /) are returned unchanged
// — filepath.Join later resolves them against the current working directory.
func expandSkillPath(path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	return path
}

// codexGlobalSkillPath returns where the Codex CLI looks for global skills.
// Honors $CODEX_HOME (Codex's own env override), defaulting to ~/.codex.
// Evaluated once at package init — if $CODEX_HOME changes mid-process we
// won't pick it up, which matches Codex's own behavior.
func codexGlobalSkillPath() string {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		return "~/.codex/skills/kestrel/SKILL.md"
	}
	return filepath.Join(codexHome, "skills", "kestrel", skillFilename)
}

// refreshSkillsIfVersionChanged is called once per CLI invocation. When the
// running binary's version differs from the sentinel at
// ~/.config/kestrel/.last-run-version, it sweeps the known skill locations
// and rewrites SKILL.md at every location that already has one. This keeps
// installed skills in sync with the binary the user just upgraded.
//
// Best-effort: never returns an error, never prints. Skipped for dev builds
// (api.Version == "dev"). The sentinel is updated only when refresh either
// fully succeeded or had nothing to do — transient write failures leave it
// stale so the next invocation retries.
//
// Hot-path cost (version unchanged): one os.ReadFile of a tiny file. Worth it
// for the upgrade-time correctness it buys.
func refreshSkillsIfVersionChanged() {
	if api.Version == "dev" {
		return
	}

	configDir, err := config.GlobalConfigDir()
	if err != nil {
		return
	}
	sentinelPath := filepath.Join(configDir, ".last-run-version")

	if data, err := os.ReadFile(sentinelPath); err == nil {
		if strings.TrimSpace(string(data)) == api.Version {
			return // up to date — fast path
		}
	}

	failed := refreshAllInstalledSkills()
	if failed > 0 {
		// Leave the sentinel stale on partial failure so the next run retries.
		return
	}

	if mkErr := os.MkdirAll(configDir, 0o755); mkErr != nil {
		return
	}
	_ = os.WriteFile(sentinelPath, []byte(api.Version), 0o644)
}

// refreshAllInstalledSkills rewrites SKILL.md at every absolute/~-prefixed
// location in skillLocations that currently has one. Project-relative paths
// are skipped — there's no reliable project root from a top-level hook.
// Returns the count of failed writes so the caller can decide whether to
// update the sentinel.
func refreshAllInstalledSkills() int {
	data, err := skills.FS.ReadFile(skillEmbeddedPath)
	if err != nil {
		return 1 // treat unreadable embed as a failure — shouldn't happen in practice
	}

	failed := 0
	refreshed := 0
	for _, loc := range skillLocations {
		// Skip project-relative paths — without a known project root, "./.claude/..."
		// could resolve to wherever the user happens to invoke the CLI.
		if !strings.HasPrefix(loc.Path, "~") && !filepath.IsAbs(loc.Path) {
			continue
		}

		expanded := expandSkillPath(loc.Path)
		if _, statErr := os.Stat(expanded); statErr != nil {
			// Not installed at this location — nothing to refresh. Permission
			// errors here are indistinguishable from "missing", so skip silently.
			continue
		}

		if writeErr := os.WriteFile(expanded, data, 0o644); writeErr == nil {
			refreshed++
		} else {
			failed++
		}
	}

	// Stamp the version sentinel inside the baseline directory too — useful
	// when someone inspects the directory directly. Only on full success.
	if refreshed > 0 && failed == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			baselineDir := filepath.Join(home, ".agents", "skills", "kestrel")
			_ = os.WriteFile(filepath.Join(baselineDir, installedVersionFile), []byte(api.Version), 0o644)
		}
	}

	return failed
}

func init() {
	skillCmd.AddCommand(skillInstallCmd)
	rootCmd.AddCommand(skillCmd)
}
