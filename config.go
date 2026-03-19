package jeff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoConfig holds per-repo configuration.
type RepoConfig struct {
	URL        string `yaml:"url"`                    // clone URL
	PostSetup  string `yaml:"post_setup,omitempty"`   // script run after worktree creation (receives src_dir, dest_dir)
}

// Config represents the jeff.yaml configuration file.
type Config struct {
	Agent    AgentTool              `yaml:"agent"`     // preferred agent tool
	GigHome string                 `yaml:"gig_home"`  // override gig home (empty = default)
	Repos    map[string]*RepoConfig `yaml:"repos"`     // name → repo config
	Home     string                 `yaml:"-"`         // resolved JEFF_HOME (not persisted in yaml)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Agent: AgentClaudeCode,
		Repos: make(map[string]*RepoConfig),
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
	// 1. Environment variable.
	if env := os.Getenv("JEFF_HOME"); env != "" {
		return env, nil
	}

	// 2. Global pointer file.
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

	// 3. Default.
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

// ConfigPath returns the jeff.yaml path within a JEFF_HOME.
func ConfigPath(jeffHome string) string {
	return filepath.Join(jeffHome, "jeff.yaml")
}

// LoadConfig reads and parses jeff.yaml from the given JEFF_HOME.
// Returns DefaultConfig if the file doesn't exist.
func LoadConfig(jeffHome string) (*Config, error) {
	path := ConfigPath(jeffHome)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.Home = jeffHome
			return &cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if !cfg.Agent.IsValid() {
		cfg.Agent = AgentClaudeCode
	}
	if cfg.Repos == nil {
		cfg.Repos = make(map[string]*RepoConfig)
	}
	cfg.Home = jeffHome
	return &cfg, nil
}

// SaveConfig writes the config to jeff.yaml.
func SaveConfig(cfg *Config) error {
	path := ConfigPath(cfg.Home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
