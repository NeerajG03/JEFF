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
