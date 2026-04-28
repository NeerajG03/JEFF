package jeff

import (
	"sort"
	"sync"
)

// LaunchOpts provides parameters for building agent launch commands.
type LaunchOpts struct {
	Model           string
	ResumeSessionID string
	Prompt          string
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
	// Returns nil if the agent doesn't support non-interactive mode.
	BuildCurateArgs(prompt string) []string

	// SupportsInlinePrompt reports whether the agent accepts a prompt
	// as a trailing positional arg in interactive mode.
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

	// EnsureHomeDirs creates agent-specific directories under JEFF_HOME.
	EnsureHomeDirs(home string) error

	// WriteHomeDefaults writes default config files (e.g. settings.json) under JEFF_HOME.
	// Idempotent — does not overwrite existing files.
	WriteHomeDefaults(home string) error

	// HookDeliveryKey returns the key used to look up the hook Delivery for this agent.
	HookDeliveryKey() string
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
