package jeff

import (
	"os"
	"path/filepath"
)

// geminiProvider implements AgentProvider for Gemini CLI.
type geminiProvider struct{}

func init() {
	RegisterProvider(&geminiProvider{})
}

func (g *geminiProvider) Name() AgentTool    { return AgentGemini }
func (g *geminiProvider) Command() string    { return "gemini" }

func (g *geminiProvider) BuildLaunchArgs(opts LaunchOpts) []string {
	args := []string{"--approval-mode=yolo"}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", "latest")
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

func (g *geminiProvider) BuildCurateArgs(prompt string) []string {
	return []string{"--approval-mode=yolo", "-p", prompt}
}

func (g *geminiProvider) SupportsInlinePrompt() bool { return true }
func (g *geminiProvider) ConfigDir() string           { return ".gemini" }
func (g *geminiProvider) SkillsSubdir() string        { return "skills" }
func (g *geminiProvider) CommandsSubdir() string      { return "commands" }
func (g *geminiProvider) CommandFileExt() string      { return "toml" }

func (g *geminiProvider) ContextFileAliases() []string {
	return []string{"GEMINI.md"}
}

func (g *geminiProvider) EnsureHomeDirs(home string) error {
	return os.MkdirAll(filepath.Join(home, ".gemini"), 0o755)
}

func (g *geminiProvider) WriteHomeDefaults(home string) error {
	writeIfNotExists(filepath.Join(home, ".gemini", "settings.json"),
		`{"$schema":"https://json.schemastore.org/gemini-settings.json"}`+"\n")
	writeIfNotExists(filepath.Join(home, ".gemini", "settings.local.json"), "{}\n")
	return nil
}

func (g *geminiProvider) HookDeliveryKey() string { return "gemini" }
