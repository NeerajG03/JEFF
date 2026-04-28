package jeff

import (
	"os"
	"path/filepath"
)

// opencodeProvider implements AgentProvider for OpenCode.
type opencodeProvider struct{}

func init() {
	RegisterProvider(&opencodeProvider{})
}

func (o *opencodeProvider) Name() AgentTool    { return AgentOpenCode }
func (o *opencodeProvider) Command() string    { return "opencode" }

func (o *opencodeProvider) BuildLaunchArgs(opts LaunchOpts) []string {
	if opts.Prompt != "" {
		return []string{"--prompt", opts.Prompt}
	}
	return nil
}

func (o *opencodeProvider) BuildCurateArgs(prompt string) []string {
	return nil // not supported
}

func (o *opencodeProvider) SupportsInlinePrompt() bool { return true }
func (o *opencodeProvider) ConfigDir() string           { return ".opencode" }
func (o *opencodeProvider) SkillsSubdir() string        { return "" }
func (o *opencodeProvider) CommandsSubdir() string      { return "" }
func (o *opencodeProvider) CommandFileExt() string      { return "" }
func (o *opencodeProvider) ContextFileAliases() []string { return nil }

func (o *opencodeProvider) EnsureHomeDirs(home string) error {
	return os.MkdirAll(filepath.Join(home, ".opencode"), 0o755)
}

func (o *opencodeProvider) WriteHomeDefaults(home string) error {
	writeIfNotExists(filepath.Join(home, ".opencode", "settings.json"), "{}\n")
	writeIfNotExists(filepath.Join(home, ".opencode", "settings.local.json"), "{}\n")
	return nil
}

func (o *opencodeProvider) HookDeliveryKey() string { return "opencode" }
