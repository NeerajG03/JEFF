package hooks

import "path/filepath"

// codexDelivery installs hooks as bash scripts + Codex hooks.json entries.
// Codex CLI uses the same JSON hook structure as Claude Code and identical
// event names (SessionStart, PostToolUse, Stop, etc.).
type codexDelivery struct{}

func init() {
	RegisterDelivery(&codexDelivery{})
}

func (d *codexDelivery) ScriptKey() string { return "codex" }

func (d *codexDelivery) Install(h *Hook, targetDir string, ctx HookContext) error {
	gen := h.Scripts[d.ScriptKey()]
	if gen == nil {
		return nil
	}
	return installClaudeScript(h, targetDir, ctx, gen, codexHooksPath(targetDir))
}

func (d *codexDelivery) Uninstall(name, targetDir string) error {
	return uninstallClaudeScript(name, targetDir, codexHooksPath(targetDir))
}

func (d *codexDelivery) SyncAll(enabled []*Hook, targetDir string, ctx HookContext) error {
	return nil // Codex installs individually like Claude.
}

func (d *codexDelivery) Installed(targetDir string) []string {
	return installedClaudeHooks(targetDir)
}

func (d *codexDelivery) IsManaged(targetDir, name string) bool {
	return scriptHasVersionMarker(scriptPath(targetDir, name))
}

func (d *codexDelivery) EventName(h *Hook) string {
	if h == nil {
		return ""
	}
	return h.Event
}

// codexHooksPath returns the .codex/hooks.json path.
func codexHooksPath(targetDir string) string {
	return filepath.Join(targetDir, ".codex", "hooks.json")
}
