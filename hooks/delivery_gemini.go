package hooks

import "path/filepath"

// geminiDelivery installs hooks as bash scripts + Gemini settings.json entries.
// Gemini CLI uses the same JSON hook protocol as Claude Code but different
// event names. This delivery maps Claude event names to Gemini equivalents
// and reuses the shared installClaudeScript/uninstallClaudeScript helpers.
type geminiDelivery struct{}

func init() {
	RegisterDelivery(&geminiDelivery{})
}

func (d *geminiDelivery) ScriptKey() string { return "gemini" }

// geminiEventName maps Claude Code event names to Gemini CLI equivalents.
// SessionStart is the same in both. Unknown events pass through unchanged.
var geminiEventMap = map[string]string{
	"PostToolUse": "AfterTool",
	"Stop":        "AfterAgent",
	"PreCompact":  "PreCompress",
}

func geminiEventName(claudeEvent string) string {
	if mapped, ok := geminiEventMap[claudeEvent]; ok {
		return mapped
	}
	return claudeEvent
}

func (d *geminiDelivery) Install(h *Hook, targetDir string, ctx HookContext) error {
	gen := h.Scripts[d.ScriptKey()]
	if gen == nil {
		return nil
	}
	// Map Claude event name to Gemini equivalent and convert timeout
	// from seconds (JEFF internal) to milliseconds (Gemini CLI).
	mapped := *h
	mapped.Event = geminiEventName(h.Event)
	mapped.Timeout = h.TimeoutOrDefault() * 1000
	return installClaudeScript(&mapped, targetDir, ctx, gen, geminiSettingsPath(targetDir))
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

func (d *geminiDelivery) IsManaged(targetDir, name string) bool {
	return scriptHasVersionMarker(scriptPath(targetDir, name))
}

func (d *geminiDelivery) EventName(claudeEvent string) string {
	return geminiEventName(claudeEvent)
}

// geminiSettingsPath returns the .gemini/settings.json path.
func geminiSettingsPath(targetDir string) string {
	return filepath.Join(targetDir, ".gemini", "settings.json")
}
