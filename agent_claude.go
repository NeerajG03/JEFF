package jeff

import (
	"os"
	"path/filepath"
)

// isClaudeModel returns true if the model name belongs to the Claude family.
// Accepts aliases (sonnet, opus, haiku) and full IDs (claude-*).
func isClaudeModel(m string) bool {
	switch m {
	case "sonnet", "opus", "haiku":
		return true
	}
	return len(m) >= 7 && m[:7] == "claude-"
}

// claudeProvider implements AgentProvider for Claude Code.
type claudeProvider struct{}

func init() {
	RegisterProvider(&claudeProvider{})
}

func (c *claudeProvider) Name() AgentTool    { return AgentClaudeCode }
func (c *claudeProvider) Command() string    { return "claude" }

func (c *claudeProvider) BuildLaunchArgs(opts LaunchOpts) []string {
	args := []string{}
	if opts.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// BuildCurateArgs ignores opts.SkipPermissions: curate is a piped,
// non-interactive run, so permissions are always skipped regardless of config.
func (c *claudeProvider) BuildCurateArgs(prompt string, opts LaunchOpts) []string {
	return []string{"--dangerously-skip-permissions", "-p", prompt}
}

func (c *claudeProvider) SupportsInlinePrompt() bool { return true }
func (c *claudeProvider) ConfigDir() string           { return ".claude" }
func (c *claudeProvider) SkillsSubdir() string        { return "skills" }
func (c *claudeProvider) CommandsSubdir() string      { return "commands" }
func (c *claudeProvider) CommandFileExt() string      { return "md" }
func (c *claudeProvider) ContextFileAliases() []string { return nil }

func (c *claudeProvider) EnsureHomeDirs(home string) error {
	return os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
}

func (c *claudeProvider) WriteHomeDefaults(home string) error {
	writeIfNotExists(filepath.Join(home, ".claude", "settings.json"),
		`{"$schema":"https://json.schemastore.org/claude-code-settings.json"}`+"\n")
	writeIfNotExists(filepath.Join(home, ".claude", "settings.local.json"), "{}\n")
	return nil
}

func (c *claudeProvider) HookDeliveryKey() string { return "claude" }

func (c *claudeProvider) InstallPersonaAgent(_, _, _, _, _ string) error { return nil }

// writeIfNotExists writes content to path only if the file doesn't already exist.
func writeIfNotExists(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	os.WriteFile(path, []byte(content), 0o644)
}
