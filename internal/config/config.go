// Package config handles loading and saving CLI configuration.
//
// Config is loaded from multiple sources with this precedence (highest wins):
//   flags > env vars > local .kestrel/config.json > global ~/.config/kestrel/config.json > defaults
//
// This is similar to how Rails uses ENV vars to override database.yml settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the resolved configuration values.
// Think of this like a Ruby Struct or Data class — it's just a bag of typed fields.
type Config struct {
	Token   string `json:"token,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// Default API base URL.
const DefaultBaseURL = "https://app.kestrelportfolio.com/api/v1"

// Load reads config from global and local files, then applies env var overrides.
// It does NOT apply flag overrides — the caller (command layer) handles those.
func Load() (*Config, error) {
	cfg := &Config{
		BaseURL: DefaultBaseURL,
	}

	// 1. Global config (~/.config/kestrel/config.json)
	globalPath, err := GlobalConfigPath()
	if err == nil {
		loadFromFile(cfg, globalPath)
	}

	// 2. Local config (.kestrel/config.json in current directory)
	localPath := filepath.Join(".kestrel", "config.json")
	loadFromFile(cfg, localPath)

	// 3. Env var overrides
	if token := os.Getenv("KESTREL_TOKEN"); token != "" {
		cfg.Token = token
	}
	if baseURL := os.Getenv("KESTREL_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	return cfg, nil
}

// GlobalConfigDir returns ~/.config/kestrel/, creating it if needed.
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "kestrel")
	return dir, nil
}

// GlobalConfigPath returns the full path to the global config file.
func GlobalConfigPath() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SaveGlobal writes config values to the global config file.
// It merges with existing values rather than overwriting the whole file.
func SaveGlobal(updates *Config) error {
	configPath, err := GlobalConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	// Load existing config to merge with
	existing := &Config{}
	loadFromFile(existing, configPath)

	// Apply updates (only non-empty fields)
	if updates.Token != "" {
		existing.Token = updates.Token
	}
	if updates.BaseURL != "" {
		existing.BaseURL = updates.BaseURL
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// loadFromFile reads a JSON config file and merges non-empty values into cfg.
// Silently returns if the file doesn't exist — this is expected for fresh installs.
func loadFromFile(cfg *Config, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file doesn't exist or unreadable — that's fine
	}

	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return // malformed JSON — skip it
	}

	if fileCfg.Token != "" {
		cfg.Token = fileCfg.Token
	}
	if fileCfg.BaseURL != "" {
		cfg.BaseURL = fileCfg.BaseURL
	}
}
