// Package harness manages integrations with AI coding agents (Claude Code, Cursor, etc.).
//
// Each agent registers itself via RegisterAgent() in an init() function in its own file.
// This is similar to Ruby's plugin pattern where each plugin calls `register` on load.
// Adding a new agent is just one new file — no changes to existing code.
//
// The registry pattern means:
//   - claude.go registers Claude Code
//   - cursor.go would register Cursor (future)
//   - windsurf.go would register Windsurf (future)
package harness

import "sync"

// StatusCheck represents a single health check result (used by `kestrel doctor`).
// Think of it like a Rails health check endpoint returning status + message.
type StatusCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pass", "warn", "fail", "skip"
	Hint   string `json:"hint,omitempty"`
}

// AgentInfo describes a coding agent integration.
type AgentInfo struct {
	Name   string                // Human-readable name, e.g. "Claude Code"
	ID     string                // Identifier used in commands, e.g. "claude"
	Detect func() bool           // Returns true if the agent is installed on this machine
	Checks func() []StatusCheck  // Health checks for `kestrel doctor`
}

var (
	registryMu sync.RWMutex
	registry   []AgentInfo
)

// RegisterAgent adds an agent to the global registry.
// Called from init() in agent-specific files (e.g., claude.go).
// Panics on empty or duplicate IDs to catch mistakes early.
func RegisterAgent(info AgentInfo) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if info.ID == "" {
		panic("harness: RegisterAgent called with empty agent ID")
	}
	for _, a := range registry {
		if a.ID == info.ID {
			panic("harness: duplicate agent ID: " + info.ID)
		}
	}
	registry = append(registry, info)
}

// AllAgents returns every registered agent.
func AllAgents() []AgentInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]AgentInfo, len(registry))
	copy(out, registry)
	return out
}

// DetectedAgents returns agents whose Detect function returns true.
// Makes a copy of the registry before calling Detect to avoid holding
// the lock during potentially slow I/O (like checking PATH).
func DetectedAgents() []AgentInfo {
	registryMu.RLock()
	snapshot := make([]AgentInfo, len(registry))
	copy(snapshot, registry)
	registryMu.RUnlock()

	var detected []AgentInfo
	for _, a := range snapshot {
		if a.Detect != nil && a.Detect() {
			detected = append(detected, a)
		}
	}
	return detected
}

// FindAgent returns the agent with the given ID, or nil if not found.
func FindAgent(id string) *AgentInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, a := range registry {
		if a.ID == id {
			info := a
			return &info
		}
	}
	return nil
}
