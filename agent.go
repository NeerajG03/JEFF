package jeff

import (
	"sort"
	"sync"
	"time"
)

type SendTiming struct {
	PasteDelay        time.Duration // between paste and Enter (0 = default 100ms)
	InterruptSettle   time.Duration // after C-c before typing (divert)
	UseBracketedPaste bool          // route via load-buffer/paste-buffer -p
}

type DoctorDep struct {
	Name     string // binary name
	Required bool
	Hint     string // install hint
}

// LaunchOpts provides parameters for building agent launch commands.
type LaunchOpts struct {
	Model           string
	ResumeSessionID string
	Prompt          string
	AgentName       string
	// SkipPermissions launches the agent with its native permission prompts
	// disabled (e.g. --dangerously-skip-permissions). Defaults to true at
	// resolution time (cmd/jeff/launch.go) to preserve current behavior.
	SkipPermissions bool
}

// AgentProvider abstracts all agent-specific CLI behavior so adding
// a new agent requires only a new provider file, not changes to every consumer.
type AgentProvider interface {
	// Name returns the AgentTool constant for this provider.
	Name() AgentTool

	// Command returns the CLI binary name (e.g. "claude", "gemini").
	Command() string

	// BuildLaunchArgs returns CLI args for an interactive session.
	BuildLaunchArgs(opts LaunchOpts) []string

	// BuildCurateArgs returns CLI args for a non-interactive (piped prompt) session.
	// Returns nil if the agent doesn't support non-interactive mode. Curate call
	// sites must always pass SkipPermissions: true — a headless curator cannot
	// answer permission prompts.
	BuildCurateArgs(prompt string, opts LaunchOpts) []string

	// SupportsInlinePrompt reports whether the agent may be launched with the
	// initial prompt baked into its argv. All current providers return false:
	// workers must run in the interactive TUI, and a launch-time prompt arg
	// pushes these CLIs into non-interactive single-turn mode (they run one turn
	// and exit, tearing down the worker). jeff always launches the bare
	// interactive CLI and pastes the prompt afterward (see crew/lifecycle.go).
	SupportsInlinePrompt() bool

	// ConfigDir returns the agent's config directory name (e.g. ".claude", ".gemini").
	ConfigDir() string

	// SkillsSubdir returns the subdirectory name for skills (e.g. "skills").
	// Returns "" if the agent doesn't support skill injection.
	SkillsSubdir() string

	// CommandsSubdir returns the subdirectory for custom commands (e.g. "commands").
	// Returns "" if the agent doesn't support custom commands.
	CommandsSubdir() string

	// CommandFileExt returns the file extension for custom commands (e.g. "md", "toml").
	// Returns "" if the agent doesn't support custom commands.
	CommandFileExt() string

	// ContextFileAliases returns extra filenames that should be symlinked to CLAUDE.md.
	// For example, gemini returns ["GEMINI.md"].
	ContextFileAliases() []string

	// ContextFileName returns the primary context filename (e.g. "CLAUDE.md", "GEMINI.md").
	ContextFileName() string

	// MemorySuppressEnv returns env assignments that disable the agent's native
	// memory, or nil when not applicable.
	MemorySuppressEnv() map[string]string

	// SendTiming returns tmux delivery tuning for this agent's TUI.
	SendTiming() SendTiming

	// OwnsModel reports whether the model alias/id belongs to this agent's family.
	OwnsModel(model string) bool

	// ModelExamples returns human-readable model names for error messages.
	ModelExamples() []string

	// DoctorDeps returns binaries this agent needs, checked by jeff doctor.
	DoctorDeps() []DoctorDep

	// EnsureHomeDirs creates agent-specific directories under JEFF_HOME.
	EnsureHomeDirs(home string) error

	// WriteHomeDefaults writes default config files (e.g. settings.json) under JEFF_HOME.
	// Idempotent — does not overwrite existing files.
	WriteHomeDefaults(home string) error

	// HookDeliveryKey returns the key used to look up the hook Delivery for this agent.
	HookDeliveryKey() string

	// InstallPersonaAgent writes a provider-native persona agent definition.
	// Providers without native persona agents should leave this as a no-op.
	InstallPersonaAgent(targetDir, name, description, model, prompt string) error
}

var (
	providersMu sync.RWMutex
	providers   = make(map[AgentTool]AgentProvider)
)

// RegisterProvider registers an AgentProvider. Panics on duplicate.
func RegisterProvider(p AgentProvider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	if _, exists := providers[p.Name()]; exists {
		panic("jeff: duplicate agent provider: " + string(p.Name()))
	}
	providers[p.Name()] = p
}

// GetProvider returns the provider for the given agent, or nil if not registered.
func GetProvider(t AgentTool) AgentProvider {
	providersMu.RLock()
	defer providersMu.RUnlock()
	return providers[t]
}

// RegisteredAgents returns all registered agent tool names, sorted.
func RegisteredAgents() []AgentTool {
	providersMu.RLock()
	defer providersMu.RUnlock()
	out := make([]AgentTool, 0, len(providers))
	for t := range providers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}
