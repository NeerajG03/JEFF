package jeff

import (
	"os"
	"path/filepath"
	"time"
)

// geminiProvider implements AgentProvider for Gemini CLI.
type geminiProvider struct{}

func init() {
	RegisterProvider(&geminiProvider{})
}

func (g *geminiProvider) Name() AgentTool { return AgentGemini }
func (g *geminiProvider) Command() string { return "gemini" }

// isGeminiModel returns true if the model name is valid for Gemini CLI.
// Accepts aliases (auto, pro, flash, flash-lite) and full IDs (gemini-*).
func isGeminiModel(m string) bool {
	switch m {
	case "auto", "pro", "flash", "flash-lite":
		return true
	}
	return len(m) >= 7 && m[:7] == "gemini-"
}

// resolveGeminiModel maps JEFF aliases to concrete model IDs where JEFF pins
// a specific version. Aliases without a pin pass through to the Gemini CLI,
// which applies its own defaults.
func resolveGeminiModel(m string) string {
	if m == "flash" {
		return "gemini-3.5-flash"
	}
	return m
}

func (g *geminiProvider) BuildLaunchArgs(opts LaunchOpts) []string {
	args := []string{}
	if opts.SkipPermissions {
		args = append(args, "--approval-mode=yolo")
	}
	if opts.Model != "" && isGeminiModel(opts.Model) {
		args = append(args, "-m", resolveGeminiModel(opts.Model))
	}
	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", "latest")
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// BuildCurateArgs ignores opts.SkipPermissions: curate is a piped,
// non-interactive run, so permissions are always skipped regardless of config.
func (g *geminiProvider) BuildCurateArgs(prompt string, opts LaunchOpts) []string {
	return []string{"--approval-mode=yolo", "-p", prompt}
}

func (g *geminiProvider) SupportsInlinePrompt() bool { return true }
func (g *geminiProvider) ConfigDir() string          { return ".gemini" }
func (g *geminiProvider) SkillsSubdir() string       { return "skills" }
func (g *geminiProvider) CommandsSubdir() string     { return "commands" }
func (g *geminiProvider) CommandFileExt() string     { return "toml" }

func (g *geminiProvider) ContextFileAliases() []string {
	return []string{"GEMINI.md"}
}

func (g *geminiProvider) ContextFileName() string { return "GEMINI.md" }
func (g *geminiProvider) MemorySuppressEnv() map[string]string {
	return map[string]string{
		"GEMINI_NO_AUTO_MEMORY": "1",
	}
}
func (g *geminiProvider) SendTiming() SendTiming {
	return SendTiming{
		PasteDelay:        500 * time.Millisecond,
		InterruptSettle:   4 * time.Second,
		UseBracketedPaste: true,
	}
}
func (g *geminiProvider) OwnsModel(model string) bool { return isGeminiModel(model) }
func (g *geminiProvider) ModelExamples() []string {
	return []string{"pro", "flash", "flash-lite", "auto", "gemini-<full-id>"}
}
func (g *geminiProvider) DoctorDeps() []DoctorDep {
	return []DoctorDep{{Name: "gemini", Required: true}}
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

func (g *geminiProvider) InstallPersonaAgent(_, _, _, _, _ string) error { return nil }
