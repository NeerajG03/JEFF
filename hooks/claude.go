package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hooksDir returns the directory where hook scripts are stored within a target dir.
func hooksDir(targetDir string) string {
	return filepath.Join(targetDir, "hooks")
}

// scriptPath returns the path for a hook's bash script.
func scriptPath(targetDir, hookName string) string {
	return filepath.Join(hooksDir(targetDir), hookName+".sh")
}

// settingsPath returns the Claude Code settings.json path.
func settingsPath(targetDir string) string {
	return filepath.Join(targetDir, ".claude", "settings.json")
}

// mkdirAll creates parent directories for the given file path.
func mkdirAll(filePath string) error {
	return os.MkdirAll(filepath.Dir(filePath), 0o755)
}

// writeExecutable writes content as an executable file.
func writeExecutable(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o755)
}

// removeFile removes a file, ignoring errors.
func removeFile(path string) {
	os.Remove(path)
}

// installedClaudeHooks returns names of hooks installed at targetDir
// by scanning the hooks/ directory for .sh files.
func installedClaudeHooks(targetDir string) []string {
	dir := hooksDir(targetDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".sh"))
	}
	return names
}

// readSettingsFile reads and parses a settings.json file.
// Returns an empty map if the file doesn't exist.
func readSettingsFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w — the file may contain comments or trailing commas (JSONC); jeff needs plain JSON here", path, err)
	}
	return settings, nil
}

// writeSettingsFile writes settings.json back to disk.
func writeSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// addHookToSettings adds a hook entry to settings under the given event.
// Idempotent: skips if the script is already present.
func addHookToSettings(settings map[string]any, event, matcher, scriptPath string, timeout int) {
	hooksRaw := settings["hooks"]
	hooksMap, _ := hooksRaw.(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	eventRaw := hooksMap[event]
	var eventBlocks []any
	if arr, ok := eventRaw.([]any); ok {
		eventBlocks = arr
	}

	// Check if already present.
	scriptName := filepath.Base(scriptPath)
	if hasHook(eventBlocks, scriptName) {
		return
	}

	block := map[string]any{
		"matcher": matcher,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": scriptPath,
				"timeout": timeout,
			},
		},
	}
	eventBlocks = append(eventBlocks, block)
	hooksMap[event] = eventBlocks
	settings["hooks"] = hooksMap
}

// removeHookFromSettings removes entries matching scriptName from all events.
func removeHookFromSettings(settings map[string]any, scriptName string) {
	hooksRaw := settings["hooks"]
	hooksMap, _ := hooksRaw.(map[string]any)
	if hooksMap == nil {
		return
	}

	for event, eventRaw := range hooksMap {
		arr, ok := eventRaw.([]any)
		if !ok {
			continue
		}

		var filtered []any
		for _, b := range arr {
			if !blockContainsScript(b, scriptName) {
				filtered = append(filtered, b)
			}
		}

		if len(filtered) == 0 {
			delete(hooksMap, event)
		} else {
			hooksMap[event] = filtered
		}
	}

	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	}
}

// hasHook checks if a script is already present in event blocks.
func hasHook(blocks []any, scriptName string) bool {
	for _, b := range blocks {
		if blockContainsScript(b, scriptName) {
			return true
		}
	}
	return false
}

// blockContainsScript checks if a matcher block references the given script.
func blockContainsScript(block any, scriptName string) bool {
	bMap, ok := block.(map[string]any)
	if !ok {
		return false
	}
	hooksList, _ := bMap["hooks"].([]any)
	for _, hk := range hooksList {
		hkMap, ok := hk.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := hkMap["command"].(string)
		for _, p := range strings.Fields(cmd) {
			if filepath.Base(p) == scriptName {
				return true
			}
		}
	}
	return false
}
