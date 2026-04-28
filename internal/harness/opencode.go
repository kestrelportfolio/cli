package harness

import (
	"os"
	"os/exec"
	"path/filepath"
)

// init registers OpenCode as an agent. Like Codex, integration is just a
// SKILL.md drop — no plugin install — so the registry entry mirrors the
// Codex shape.
func init() {
	RegisterAgent(AgentInfo{
		Name:   "OpenCode",
		ID:     "opencode",
		Detect: detectOpenCode,
		Checks: checkOpenCode,
	})
}

// opencodeConfigDir returns the conventional ~/.config/opencode location.
// OpenCode does not document an env override at the time of writing — if it
// adds one later, plumb it through here.
func opencodeConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode"), nil
}

// detectOpenCode returns true if the `opencode` binary is on PATH or the
// OpenCode config directory exists.
func detectOpenCode() bool {
	if _, err := exec.LookPath("opencode"); err == nil {
		return true
	}
	dir, err := opencodeConfigDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(dir)
	return err == nil
}

// checkOpenCode reports whether SKILL.md is installed at the location
// OpenCode reads. Note the directory is `skill/` (singular) — that's what
// OpenCode looks for.
func checkOpenCode() []StatusCheck {
	dir, err := opencodeConfigDir()
	if err != nil {
		return []StatusCheck{{
			Name:   "Kestrel skill installed for OpenCode",
			Status: "skip",
			Hint:   "could not resolve OpenCode config directory",
		}}
	}
	expected := filepath.Join(dir, "skill", "kestrel", "SKILL.md")
	if _, err := os.Stat(expected); err != nil {
		return []StatusCheck{{
			Name:   "Kestrel skill installed for OpenCode",
			Status: "fail",
			Hint:   "Run: kestrel setup opencode",
		}}
	}
	return []StatusCheck{{
		Name:   "Kestrel skill installed for OpenCode",
		Status: "pass",
	}}
}
