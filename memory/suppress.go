// suppress.go — Worker A: native-memory suppression for Claude Code and Gemini CLI.
// Sets env-var overrides and writes per-agent settings files that disable
// auto-memory so JEFF owns the memory surface.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnvOverrides returns the environment variable overrides required to suppress
// native CLI memory for the given persona and agent kind.
//
// For persona=="marlowe", JEFF_MEMORY_CAN_ADD is set to "1".
// For all other personas, JEFF_MEMORY_CAN_ADD is set to "" (explicit unset).
//
// The map is suitable for merging into an os/exec.Cmd.Env slice or for writing
// into a settings.json "env" block.
func EnvOverrides(persona, agentKind string) map[string]string {
	m := make(map[string]string)
	switch agentKind {
	case "claude":
		m["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] = "1"
	case "gemini":
		m["GEMINI_NO_AUTO_MEMORY"] = "1"
	}
	if persona == "marlowe" {
		m["JEFF_MEMORY_CAN_ADD"] = "1"
	} else {
		// Explicitly unset so the var is not inherited from a parent marlowe session.
		m["JEFF_MEMORY_CAN_ADD"] = ""
	}
	return m
}

// ApplySettings writes native-memory disable flags into the agent's settings file
// inside the task directory. Merges with any existing content; idempotent.
//
// Claude: writes CLAUDE_CODE_DISABLE_AUTO_MEMORY=1 to .claude/settings.json env block.
// Gemini: writes memory.autoload=false to .gemini/settings.json.
func ApplySettings(taskDir, agentKind string) error {
	switch agentKind {
	case "claude":
		return applyClaudeSuppress(taskDir)
	case "gemini":
		return applyGeminiSuppress(taskDir)
	default:
		return nil
	}
}

func applyClaudeSuppress(taskDir string) error {
	path := filepath.Join(taskDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("suppress: mkdir .claude: %w", err)
	}
	settings, err := readSettingsJSON(path)
	if err != nil {
		return fmt.Errorf("suppress: read .claude/settings.json: %w", err)
	}

	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = make(map[string]any)
	}
	env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] = "1"
	settings["env"] = env

	return writeSettingsJSON(path, settings)
}

func applyGeminiSuppress(taskDir string) error {
	path := filepath.Join(taskDir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("suppress: mkdir .gemini: %w", err)
	}
	settings, err := readSettingsJSON(path)
	if err != nil {
		return fmt.Errorf("suppress: read .gemini/settings.json: %w", err)
	}

	mem, _ := settings["memory"].(map[string]any)
	if mem == nil {
		mem = make(map[string]any)
	}
	mem["autoload"] = false
	settings["memory"] = mem

	return writeSettingsJSON(path, settings)
}

// readSettingsJSON reads a JSON settings file into a map.
// Returns an empty map if the file does not exist.
func readSettingsJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// writeSettingsJSON marshals m as indented JSON and writes it to path.
func writeSettingsJSON(path string, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "    ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
