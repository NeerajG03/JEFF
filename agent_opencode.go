package jeff

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// opencodeProvider implements AgentProvider for OpenCode.
type opencodeProvider struct{}

func init() {
	RegisterProvider(&opencodeProvider{})
}

func (o *opencodeProvider) Name() AgentTool    { return AgentOpenCode }
func (o *opencodeProvider) Command() string    { return "opencode" }

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

func (o *opencodeProvider) SupportsInlinePrompt() bool { return true }
func (o *opencodeProvider) ConfigDir() string           { return ".opencode" }
func (o *opencodeProvider) SkillsSubdir() string        { return "skills" }
func (o *opencodeProvider) CommandsSubdir() string      { return "commands" }
func (o *opencodeProvider) CommandFileExt() string      { return "md" }
func (o *opencodeProvider) ContextFileAliases() []string { return []string{"AGENTS.md"} }

func (o *opencodeProvider) EnsureHomeDirs(home string) error {
	for _, dir := range []string{"agents", "commands", "plugins", "skills"} {
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

// InstallPersonaAgent writes a task/home-local OpenCode agent definition.
// The file is managed by JEFF but existing files are preserved so users can
// customize a persona without an update unexpectedly overwriting it.
func (o *opencodeProvider) InstallPersonaAgent(targetDir, name, description, model, prompt string) error {
	if name == "" || prompt == "" {
		return nil
	}
	if description == "" {
		description = name + " persona"
	}
	var content strings.Builder
	content.WriteString("---\n")
	fmt.Fprintf(&content, "description: %s\n", strconv.Quote(description))
	content.WriteString("mode: all\n")
	if isOpenCodeModel(model) {
		fmt.Fprintf(&content, "model: %s\n", model)
	}
	content.WriteString("---\n\n")
	content.WriteString(prompt)
	if !strings.HasSuffix(prompt, "\n") {
		content.WriteByte('\n')
	}

	path := filepath.Join(targetDir, ".opencode", "agents", name+".md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create OpenCode agents dir: %w", err)
	}
	return os.WriteFile(path, []byte(content.String()), 0o644)
}
