package jeff

import "slices"

// AgentTool represents a supported agent CLI tool.
type AgentTool string

const (
	AgentClaudeCode AgentTool = "claude"
	AgentOpenCode   AgentTool = "opencode"
	AgentGemini     AgentTool = "gemini"
)

// ValidAgentTools is the set of supported agent tools.
// Kept for backward compatibility; derived from the provider registry.
var ValidAgentTools = []AgentTool{AgentClaudeCode, AgentOpenCode, AgentGemini}

// IsValid returns true if t is a recognized agent tool.
func (t AgentTool) IsValid() bool {
	return slices.Contains(ValidAgentTools, t)
}

// ValidNames returns the valid agent tool names as strings.
func (AgentTool) ValidNames() []string {
	names := make([]string, len(ValidAgentTools))
	for i, t := range ValidAgentTools {
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
