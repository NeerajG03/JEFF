package hooks

import (
	"fmt"
	"strings"
)

// claudeDelivery installs hooks as bash scripts + Claude Code settings.json entries.
type claudeDelivery struct{}

func init() {
	RegisterDelivery(&claudeDelivery{})
}

func (d *claudeDelivery) ScriptKey() string { return "claude" }

func (d *claudeDelivery) Install(h *Hook, targetDir string, ctx HookContext) error {
	gen := h.Scripts[d.ScriptKey()]
	if gen == nil {
		return nil
	}
	return installClaudeScript(h, targetDir, ctx, gen, claudeSettingsPath(targetDir))
}

func (d *claudeDelivery) Uninstall(name, targetDir string) error {
	return uninstallClaudeScript(name, targetDir, claudeSettingsPath(targetDir))
}

func (d *claudeDelivery) SyncAll(enabled []*Hook, targetDir string, ctx HookContext) error {
	return nil // Claude installs individually; no batch step needed.
}

func (d *claudeDelivery) Installed(targetDir string) []string {
	return installedClaudeHooks(targetDir)
}

// claudeSettingsPath returns the .claude/settings.json path.
func claudeSettingsPath(targetDir string) string {
	return settingsPath(targetDir)
}

// installClaudeScript writes the bash script and updates a settings.json file.
// Extracted from installClaude to support reuse by geminiDelivery.
func installClaudeScript(h *Hook, targetDir string, ctx HookContext, gen func(HookContext) string, settingsFile string) error {
	content := gen(ctx)
	if strings.HasPrefix(content, "#!/bin/bash\n") {
		content = "#!/bin/bash\n# jeff-hook-version: " + ScriptVersion + "\n" + strings.TrimPrefix(content, "#!/bin/bash\n")
	}
	sp := scriptPath(targetDir, h.Name)

	if err := mkdirAll(sp); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}
	if err := writeExecutable(sp, content); err != nil {
		return fmt.Errorf("write script %s: %w", sp, err)
	}

	settings, err := readSettingsFile(settingsFile)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	addHookToSettings(settings, h.Event, h.Matcher, sp, h.TimeoutOrDefault())

	if err := writeSettingsFile(settingsFile, settings); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

// uninstallClaudeScript removes a hook's script and settings.json entry.
func uninstallClaudeScript(name, targetDir, settingsFile string) error {
	sp := scriptPath(targetDir, name)

	settings, err := readSettingsFile(settingsFile)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	removeHookFromSettings(settings, name+".sh")

	if err := writeSettingsFile(settingsFile, settings); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	removeFile(sp)
	return nil
}
