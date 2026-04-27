package hooks

import "path/filepath"

// geminiDelivery installs hooks as bash scripts + Gemini settings.json entries.
// Gemini CLI uses the same JSON hook protocol as Claude Code, so this delivery
// reuses the shared installClaudeScript/uninstallClaudeScript helpers with a
// different settings.json path.
type geminiDelivery struct{}

func init() {
	RegisterDelivery(&geminiDelivery{})
}

func (d *geminiDelivery) ScriptKey() string { return "gemini" }

func (d *geminiDelivery) Install(h *Hook, targetDir string, ctx HookContext) error {
	gen := h.Scripts[d.ScriptKey()]
	if gen == nil {
		return nil
	}
	return installClaudeScript(h, targetDir, ctx, gen, geminiSettingsPath(targetDir))
}

func (d *geminiDelivery) Uninstall(name, targetDir string) error {
	return uninstallClaudeScript(name, targetDir, geminiSettingsPath(targetDir))
}

func (d *geminiDelivery) SyncAll(enabled []*Hook, targetDir string, ctx HookContext) error {
	return nil // Gemini installs individually like Claude.
}

func (d *geminiDelivery) Installed(targetDir string) []string {
	// Gemini shares the same hooks/ directory as Claude (script files).
	return installedClaudeHooks(targetDir)
}

// geminiSettingsPath returns the .gemini/settings.json path.
func geminiSettingsPath(targetDir string) string {
	return filepath.Join(targetDir, ".gemini", "settings.json")
}
