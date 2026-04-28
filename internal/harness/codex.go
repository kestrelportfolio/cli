package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// init registers the OpenAI Codex CLI as an agent. Unlike Claude Code, Codex
// has no plugin/marketplace concept — integration is just dropping SKILL.md
// in its conventional location, so the registry only carries Detect + Checks.
func init() {
	RegisterAgent(AgentInfo{
		Name:   "Codex",
		ID:     "codex",
		Detect: detectCodex,
		Checks: checkCodex,
	})
}

// codexHomeDir resolves the Codex install root. Honors $CODEX_HOME (Codex's
// own override), else defaults to ~/.codex.
func codexHomeDir() (string, error) {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return codexHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// detectCodex returns true if the `codex` binary is on PATH or its home
// directory exists. The directory check catches users who installed Codex
// via the IDE extension and don't have the CLI shimmed.
func detectCodex() bool {
	if _, err := exec.LookPath("codex"); err == nil {
		return true
	}
	dir, err := codexHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(dir)
	return err == nil
}

// checkCodex reports whether SKILL.md is installed at the location Codex
// reads. Single check — there's no plugin to verify here.
func checkCodex() []StatusCheck {
	dir, err := codexHomeDir()
	if err != nil {
		return []StatusCheck{{
			Name:   "Kestrel skill installed for Codex",
			Status: "skip",
			Hint:   "could not resolve Codex home directory",
		}}
	}
	expected := filepath.Join(dir, "skills", "kestrel", "SKILL.md")
	if _, err := os.Stat(expected); err != nil {
		return []StatusCheck{{
			Name:   "Kestrel skill installed for Codex",
			Status: "fail",
			Hint:   "Run: kestrel setup codex",
		}}
	}
	return []StatusCheck{{
		Name:   "Kestrel skill installed for Codex",
		Status: "pass",
	}}
}
