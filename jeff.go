package jeff

import (
	"slices"
	"strings"
)

// AgentTool represents a supported agent CLI tool.
type AgentTool string

const (
	AgentClaudeCode AgentTool = "claude"
	AgentOpenCode   AgentTool = "opencode"
	AgentGemini     AgentTool = "gemini"
)

// InferBackend returns the AgentTool for a model name based on known model families.
// Returns "" if model is empty or does not belong to any known family.
// claude family: sonnet, opus, haiku, claude-*
// gemini family: pro, flash, flash-lite, auto, gemini-*
func InferBackend(model string) AgentTool {
	if model == "" {
		return ""
	}
	if isClaudeModel(model) {
		return AgentClaudeCode
	}
	if isGeminiModel(model) {
		return AgentGemini
	}
	return ""
}

// IsValidModel reports whether model is a known model for the given agent.
func IsValidModel(agent AgentTool, model string) bool {
	switch agent {
	case AgentClaudeCode:
		return isClaudeModel(model)
	case AgentGemini:
		return isGeminiModel(model)
	}
	return false
}

// ValidModelsForBackend returns the human-readable model names for an agent (for error messages).
func ValidModelsForBackend(agent AgentTool) []string {
	switch agent {
	case AgentClaudeCode:
		return []string{"sonnet", "opus", "haiku", "claude-<full-id>"}
	case AgentGemini:
		return []string{"pro", "flash", "flash-lite", "auto", "gemini-<full-id>"}
	}
	return nil
}

// UnknownModelError returns an error message for an unrecognized model name.
func UnknownModelError(model string) string {
	return "unknown model " + `"` + model + `"` +
		"\nValid Claude models: " + strings.Join(ValidModelsForBackend(AgentClaudeCode), ", ") +
		"\nValid Gemini models: " + strings.Join(ValidModelsForBackend(AgentGemini), ", ")
}

// IsValid returns true if t is a recognized agent tool (has a registered provider).
func (t AgentTool) IsValid() bool {
	return GetProvider(t) != nil
}

// ValidNames returns the valid agent tool names as strings, derived from the provider registry.
func (AgentTool) ValidNames() []string {
	agents := RegisteredAgents()
	names := make([]string, len(agents))
	for i, t := range agents {
		names[i] = string(t)
	}
	return names
}

// Command returns the CLI command name used to launch this agent tool.
// Delegates to the registered provider if available.
func (t AgentTool) Command() string {
	if p := GetProvider(t); p != nil {
		return p.Command()
	}
	return string(t)
}

// IDE represents a supported code editor / IDE.
type IDE string

const (
	IDEVSCode   IDE = "vscode"
	IDECursor   IDE = "cursor"
	IDEWindsurf IDE = "windsurf"
	IDENvim     IDE = "nvim"
	IDEZed      IDE = "zed"
)

// ValidIDEs is the set of supported IDEs.
var ValidIDEs = []IDE{IDEVSCode, IDECursor, IDEWindsurf, IDENvim, IDEZed}

// IsValid returns true if i is a recognized IDE.
func (i IDE) IsValid() bool {
	return slices.Contains(ValidIDEs, i)
}

// ValidNames returns the valid IDE names as strings.
func (IDE) ValidNames() []string {
	names := make([]string, len(ValidIDEs))
	for i, ide := range ValidIDEs {
		names[i] = string(ide)
	}
	return names
}

// Terminal reports whether this IDE runs inside the terminal (TUI)
// rather than launching a separate GUI window.
func (i IDE) Terminal() bool {
	switch i {
	case IDENvim:
		return true
	default:
		return false
	}
}

// Command returns the CLI command used to open this IDE.
func (i IDE) Command() string {
	switch i {
	case IDEVSCode:
		return "code"
	case IDECursor:
		return "cursor"
	case IDEWindsurf:
		return "windsurf"
	case IDENvim:
		return "nvim"
	case IDEZed:
		return "zed"
	default:
		return string(i)
	}
}
