package hooks

import "fmt"

// AgentTool mirrors jeff.AgentTool to avoid circular imports.
type AgentTool string

const (
	AgentClaudeCode AgentTool = "claude"
	AgentOpenCode   AgentTool = "opencode"
)

// Manager orchestrates hook installation and removal.
type Manager struct {
	registry *Registry
}

// NewManager creates a manager backed by the given registry.
func NewManager(r *Registry) *Manager {
	return &Manager{registry: r}
}

// Install writes a single hook's artifacts to targetDir.
func (m *Manager) Install(name, targetDir string, agent AgentTool, ctx HookContext) error {
	h := m.registry.Get(name)
	if h == nil {
		return fmt.Errorf("hook %q not found", name)
	}

	if agent == AgentClaudeCode && h.ClaudeScript != nil {
		if err := installClaude(h, targetDir, ctx); err != nil {
			return fmt.Errorf("install claude hook %s: %w", name, err)
		}
	}
	// OpenCode is handled by syncOpenCode in Sync (single file for all hooks).
	return nil
}

// Uninstall removes a single hook's artifacts from targetDir.
func (m *Manager) Uninstall(name, targetDir string, agent AgentTool) error {
	if agent == AgentClaudeCode {
		if err := uninstallClaude(name, targetDir); err != nil {
			return fmt.Errorf("uninstall claude hook %s: %w", name, err)
		}
	}
	// OpenCode is handled by syncOpenCode in Sync.
	return nil
}

// Sync ensures targetDir has exactly the hooks in `enabled`.
// Installs missing hooks, uninstalls extra ones. Idempotent.
func (m *Manager) Sync(targetDir string, enabled map[string]bool, agent AgentTool, ctx HookContext) error {
	// Current state.
	installed := make(map[string]bool)
	for _, name := range m.Installed(targetDir) {
		installed[name] = true
	}

	// Install or update all enabled hooks (always overwrite scripts
	// so content updates propagate on sync).
	for name := range enabled {
		if err := m.Install(name, targetDir, agent, ctx); err != nil {
			return err
		}
	}

	// Uninstall extra (only hooks known to our registry).
	for name := range installed {
		if !enabled[name] && m.registry.Get(name) != nil {
			if err := m.Uninstall(name, targetDir, agent); err != nil {
				return err
			}
		}
	}

	// Regenerate OpenCode plugin from enabled hooks.
	if agent == AgentOpenCode {
		var ocHooks []*Hook
		for name := range enabled {
			if h := m.registry.Get(name); h != nil && h.OpenCodeSnippet != nil {
				ocHooks = append(ocHooks, h)
			}
		}
		if err := syncOpenCode(ocHooks, targetDir, ctx); err != nil {
			return fmt.Errorf("sync opencode hooks: %w", err)
		}
	}

	return nil
}

// Installed returns names of hooks currently installed at targetDir.
func (m *Manager) Installed(targetDir string) []string {
	return installedClaudeHooks(targetDir)
}
