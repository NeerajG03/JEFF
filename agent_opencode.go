package jeff

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// opencodeProvider implements AgentProvider for OpenCode.
type opencodeProvider struct{}

func init() {
	RegisterProvider(&opencodeProvider{})
}

var (
	openCodeAliasesMu sync.RWMutex
	openCodeAliases   map[string]string // name -> actualModel
)

// SetOpenCodeModelAliases installs the user's configured opencode model
// registry (Config.OpenCodeModels). Called once at startup after config
// load (cmd/jeff/main.go), and after any config mutation (opencode_models.go).
func SetOpenCodeModelAliases(aliases map[string]string) {
	openCodeAliasesMu.Lock()
	defer openCodeAliasesMu.Unlock()
	openCodeAliases = aliases
}

// resolveOpenCodeAlias returns the actual provider/model id for a registered
// name, or ("", false) if model isn't a registered name.
func resolveOpenCodeAlias(model string) (string, bool) {
	openCodeAliasesMu.RLock()
	defer openCodeAliasesMu.RUnlock()
	actual, ok := openCodeAliases[model]
	return actual, ok
}

// isOpenCodeConfiguredActual reports whether model equals a registered
// actual provider/model id.
func isOpenCodeConfiguredActual(model string) bool {
	openCodeAliasesMu.RLock()
	defer openCodeAliasesMu.RUnlock()
	for _, actual := range openCodeAliases {
		if actual == model {
			return true
		}
	}
	return false
}

// openCodeRegistryEmpty reports whether the user has registered any opencode
// model aliases at all.
func openCodeRegistryEmpty() bool {
	openCodeAliasesMu.RLock()
	defer openCodeAliasesMu.RUnlock()
	return len(openCodeAliases) == 0
}

// openCodeRegisteredNames returns the sorted list of registered alias names.
func openCodeRegisteredNames() []string {
	openCodeAliasesMu.RLock()
	defer openCodeAliasesMu.RUnlock()
	names := make([]string, 0, len(openCodeAliases))
	for name := range openCodeAliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	resolvedModel := opts.Model
	if actual, ok := resolveOpenCodeAlias(opts.Model); ok {
		resolvedModel = actual
	}
	if isOpenCodeModel(resolvedModel) {
		args = append(args, "--model", resolvedModel)
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

// OwnsModel recognizes a model as OpenCode's if it matches a registered
// alias name or actual provider/model id. If no aliases are registered at
// all, it falls back to the structural isOpenCodeModel check so opencode
// remains usable with zero configuration. Once at least one alias is
// registered, only registered names/actuals are recognized.
func (o *opencodeProvider) OwnsModel(model string) bool {
	if _, ok := resolveOpenCodeAlias(model); ok {
		return true
	}
	if isOpenCodeConfiguredActual(model) {
		return true
	}
	if openCodeRegistryEmpty() {
		return isOpenCodeModel(model)
	}
	return false
}

func (o *opencodeProvider) ModelExamples() []string {
	examples := []string{"provider/model"}
	return append(examples, openCodeRegisteredNames()...)
}
func (o *opencodeProvider) DoctorDeps() []DoctorDep {
	return []DoctorDep{{Name: "opencode", Required: false}}
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
