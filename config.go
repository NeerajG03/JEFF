package jeff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configSchemaURL = "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/jeff-config.json"

// RepoConfig holds per-repo configuration.
type RepoConfig struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	BaseBranch  string `json:"base_branch,omitempty" yaml:"base_branch,omitempty"`
	BranchName  string `json:"branch_name,omitempty" yaml:"branch_name,omitempty"`
	PostSetup   string `json:"post_setup,omitempty" yaml:"post_setup,omitempty"`
}

// MemoryConfig holds memory-subsystem configuration persisted in jeff.json.
type MemoryConfig struct {
	Disabled bool `json:"disabled,omitempty"`
}

// Config represents the jeff.json configuration file.
type Config struct {
	Schema             string                 `json:"$schema,omitempty" yaml:"-"`
	Agent              AgentTool              `json:"agent" yaml:"agent"`
	IDE                IDE                    `json:"ide,omitempty" yaml:"ide,omitempty"`
	GigHome            string                 `json:"gig_home,omitempty" yaml:"gig_home"`
	GigPrefix          string                 `json:"gig_prefix,omitempty" yaml:"gig_prefix,omitempty"`
	Repos              map[string]*RepoConfig `json:"repos" yaml:"repos"`
	Hooks              map[string]bool        `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	CheckpointPatterns []string               `json:"checkpoint_patterns,omitempty" yaml:"checkpoint_patterns,omitempty"`
	Memory             *MemoryConfig          `json:"memory,omitempty" yaml:"memory,omitempty"`
	// OpenCodeModels maps a user-chosen --model alias (name) to the real
	// OpenCode provider/model id (actual). See opencode_models.go.
	OpenCodeModels map[string]string `json:"opencode_models,omitempty" yaml:"-"`
	// SkipPermissions controls whether agents launch with their native
	// permission prompts disabled. Pointer so "unset" (nil → default true,
	// current behavior) is distinguishable from an explicit false.
	SkipPermissions *bool  `json:"skip_permissions,omitempty" yaml:"skip_permissions,omitempty"`
	Home            string `json:"-" yaml:"-"` // resolved JEFF_HOME (not persisted)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Schema: configSchemaURL,
		Agent:  AgentClaudeCode,
		Repos:  make(map[string]*RepoConfig),
	}
}

// globalPointerPath returns ~/.config/jeff/home which stores the JEFF_HOME path.
func globalPointerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".config", "jeff", "home"), nil
}

// ResolveHome determines the JEFF_HOME directory.
// Resolution order: JEFF_HOME env var → global pointer file → ~/.jeff/
func ResolveHome() (string, error) {
	if env := os.Getenv("JEFF_HOME"); env != "" {
		return env, nil
	}

	ptr, err := globalPointerPath()
	if err == nil {
		data, err := os.ReadFile(ptr)
		if err == nil {
			p := strings.TrimSpace(string(data))
			if p != "" {
				return p, nil
			}
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".jeff"), nil
}

// WriteHomePointer writes the JEFF_HOME path to the global pointer file.
func WriteHomePointer(jeffHome string) error {
	ptr, err := globalPointerPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ptr), 0o755); err != nil {
		return fmt.Errorf("create pointer dir: %w", err)
	}
	return os.WriteFile(ptr, []byte(jeffHome+"\n"), 0o644)
}

// ConfigPath returns the jeff.json path within a JEFF_HOME.
func ConfigPath(jeffHome string) string {
	return filepath.Join(jeffHome, "jeff.json")
}

// legacyConfigPath returns the old jeff.yaml path.
func legacyConfigPath(jeffHome string) string {
	return filepath.Join(jeffHome, "jeff.yaml")
}

// LoadConfig reads and parses jeff.json from the given JEFF_HOME.
// If jeff.json is missing but jeff.yaml exists, it auto-migrates.
// Returns DefaultConfig if neither file exists.
func LoadConfig(jeffHome string) (*Config, error) {
	path := ConfigPath(jeffHome)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Try legacy yaml.
			cfg, migrated := migrateFromYAML(jeffHome)
			if migrated {
				return cfg, nil
			}
			cfg2 := DefaultConfig()
			cfg2.Home = jeffHome
			return &cfg2, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if !cfg.Agent.IsValid() {
		cfg.Agent = AgentClaudeCode
	}
	if cfg.IDE != "" && !cfg.IDE.IsValid() {
		cfg.IDE = IDEVSCode
	}
	if cfg.Repos == nil {
		cfg.Repos = make(map[string]*RepoConfig)
	}
	cfg.Home = jeffHome
	return &cfg, nil
}

// migrateFromYAML reads jeff.yaml, converts to jeff.json, and removes the yaml file.
// Returns the config and true if migration happened.
func migrateFromYAML(jeffHome string) (*Config, bool) {
	yamlPath := legacyConfigPath(jeffHome)
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, false
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}

	if !cfg.Agent.IsValid() {
		cfg.Agent = AgentClaudeCode
	}
	if cfg.Repos == nil {
		cfg.Repos = make(map[string]*RepoConfig)
	}
	cfg.Schema = configSchemaURL
	cfg.Home = jeffHome

	// Write jeff.json.
	if err := SaveConfig(&cfg); err != nil {
		return nil, false
	}

	// Remove legacy file.
	os.Remove(yamlPath)

	return &cfg, true
}

// SaveConfig writes the config to jeff.json.
func SaveConfig(cfg *Config) error {
	path := ConfigPath(cfg.Home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if cfg.Schema == "" {
		cfg.Schema = configSchemaURL
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o644)
}
