package jeff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// opencodeProvider implements AgentProvider for OpenCode.
type opencodeProvider struct{}

func init() {
	RegisterProvider(&opencodeProvider{})
}

func (o *opencodeProvider) Name() AgentTool { return AgentOpenCode }
func (o *opencodeProvider) Command() string { return "opencode" }

func (o *opencodeProvider) BuildLaunchArgs(opts LaunchOpts) []string {
	// --auto is OpenCode's permission-skip equivalent (auto-accepts edits
	// without prompting) — gate it the same as the other providers' flags.
	args := []string{}
	if opts.SkipPermissions {
		args = append(args, "--auto")
	}
	if isOpenCodeModel(opts.Model) {
		args = append(args, "--model", opts.Model)
	}
	// Intentionally ignore opts.AgentName: JEFF personas are not OpenCode
	// agents. OpenCode resolves --agent against .opencode/agents/, which JEFF
	// does not populate in task dirs, so passing it selects a nonexistent
	// agent. Persona context reaches OpenCode via CLAUDE.md instead.
	if opts.ResumeSessionID != "" {
		args = append(args, "--session", opts.ResumeSessionID)
	}
	if opts.Prompt != "" {
		args = append(args, "--prompt", opts.Prompt)
	}
	return args
}

// BuildCurateArgs ignores opts.SkipPermissions: curate is a piped,
// non-interactive run, so permissions are always skipped regardless of config.
func (o *opencodeProvider) BuildCurateArgs(prompt string, opts LaunchOpts) []string {
	return []string{"run", "--auto", prompt}
}

// SupportsInlinePrompt is false: workers must always run in the interactive TUI
// rather than being launched with a baked-in --prompt. jeff pastes the initial
// prompt after launch (see lifecycle.go).
func (o *opencodeProvider) SupportsInlinePrompt() bool   { return false }
func (o *opencodeProvider) ConfigDir() string            { return ".opencode" }
func (o *opencodeProvider) SkillsSubdir() string         { return "skills" }
func (o *opencodeProvider) CommandsSubdir() string       { return "commands" }
func (o *opencodeProvider) CommandFileExt() string       { return "md" }
func (o *opencodeProvider) ContextFileAliases() []string { return []string{"AGENTS.md"} }

func (o *opencodeProvider) ContextFileName() string { return "CLAUDE.md" }
func (o *opencodeProvider) MemorySuppressEnv() map[string]string {
	return nil
}
func (o *opencodeProvider) SendTiming() SendTiming {
	return SendTiming{
		PasteDelay:        100 * time.Millisecond,
		InterruptSettle:   2 * time.Second,
		UseBracketedPaste: false,
	}
}
func (o *opencodeProvider) OwnsModel(model string) bool { return false }
func (o *opencodeProvider) ModelExamples() []string {
	return []string{"provider/model"}
}
func (o *opencodeProvider) DoctorDeps() []DoctorDep {
	return []DoctorDep{{Name: "opencode", Required: true}}
}

func (o *opencodeProvider) EnsureHomeDirs(home string) error {
	for _, dir := range []string{"commands", "plugins", "skills"} {
		if err := os.MkdirAll(filepath.Join(home, ".opencode", dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (o *opencodeProvider) WriteHomeDefaults(home string) error {
	opencodeDir := filepath.Join(home, ".opencode")
	writeIfNotExists(filepath.Join(opencodeDir, "opencode.json"),
		`{"$schema":"https://opencode.ai/config.json","instructions":["CLAUDE.md"]}`+"\n")

	// Remove stale config files from before opencode.json was adopted.
	for _, stale := range []string{"settings.json", "settings.local.json"} {
		if err := os.Remove(filepath.Join(opencodeDir, stale)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", stale, err)
		}
	}
	return nil
}

func (o *opencodeProvider) HookDeliveryKey() string { return "opencode" }

// isOpenCodeModel accepts OpenCode's provider/model identifiers. Bare Claude
// aliases are intentionally ignored so they fall back to OpenCode's configured
// model instead of producing an invalid OpenCode invocation.
func isOpenCodeModel(model string) bool {
	provider, modelID, ok := strings.Cut(model, "/")
	return ok && provider != "" && modelID != ""
}
