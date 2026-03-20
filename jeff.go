package jeff

// AgentTool represents a supported agent CLI tool.
type AgentTool string

const (
	AgentClaudeCode AgentTool = "claude"
	AgentOpenCode   AgentTool = "opencode"
)

// ValidAgentTools is the set of supported agent tools.
var ValidAgentTools = []AgentTool{AgentClaudeCode, AgentOpenCode}

// IsValid returns true if t is a recognized agent tool.
func (t AgentTool) IsValid() bool {
	for _, v := range ValidAgentTools {
		if t == v {
			return true
		}
	}
	return false
}

// Command returns the CLI command name used to launch this agent tool.
func (t AgentTool) Command() string {
	switch t {
	case AgentClaudeCode:
		return "claude"
	case AgentOpenCode:
		return "opencode"
	default:
		return string(t)
	}
}

// IDE represents a supported code editor / IDE.
type IDE string

const (
	IDEVSCode   IDE = "vscode"
	IDECursor   IDE = "cursor"
	IDEWindsurf IDE = "windsurf"
	IDENvim     IDE = "nvim"
)

// ValidIDEs is the set of supported IDEs.
var ValidIDEs = []IDE{IDEVSCode, IDECursor, IDEWindsurf, IDENvim}

// IsValid returns true if i is a recognized IDE.
func (i IDE) IsValid() bool {
	for _, v := range ValidIDEs {
		if i == v {
			return true
		}
	}
	return false
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
	default:
		return string(i)
	}
}
