// Package hooks provides a hook management system for injecting context
// into agent sessions (Claude Code and OpenCode).
package hooks

// Source identifies where a hook gets installed.
type Source string

const (
	// SourceHome hooks are installed at JEFF_HOME level (by jeff init).
	SourceHome Source = "home"
	// SourceTask hooks are installed at task directory level (by jeff pickup).
	SourceTask Source = "task"
)

// Hook defines a single injectable behavior for agent sessions.
type Hook struct {
	Name    string // unique identifier, e.g. "gig-instructions"
	Source  Source  // where this hook belongs (home or task)
	Event   string // agent event: "SessionStart", "PreCompact", "PostToolUse"
	Matcher string // event matcher: "*", "Bash"
	Timeout int    // seconds; 0 defaults to 10

	// ClaudeScript generates bash script content for Claude Code hooks.
	ClaudeScript func(ctx HookContext) string

	// OpenCodeSnippet generates a JS code snippet for the OpenCode plugin.
	OpenCodeSnippet func(ctx HookContext) string
}

// HookContext provides data that hook generators need.
type HookContext struct {
	JeffHome           string   // JEFF_HOME path
	TargetDir          string   // directory where hooks are being installed
	GigHome            string   // gig home for SDK calls (empty = default)
	TaskID             string   // gig task ID (set for task-level hooks)
	OrchestratorID     string   // orchestrator ID (set for orchestrator hooks)
	CheckpointPatterns []string // regex patterns that trigger checkpoint nudge
}

// TimeoutOrDefault returns the hook's timeout, defaulting to 10 seconds.
func (h *Hook) TimeoutOrDefault() int {
	if h.Timeout > 0 {
		return h.Timeout
	}
	return 10
}

// EnabledForSource returns the enabled hook names for a given source.
// If cfg is nil, all hooks for the source are enabled.
// Hooks not mentioned in cfg default to enabled.
func EnabledForSource(cfg map[string]bool, source Source, reg *Registry) map[string]bool {
	result := make(map[string]bool)
	for _, h := range reg.BySource(source) {
		if cfg == nil {
			result[h.Name] = true
		} else if enabled, ok := cfg[h.Name]; !ok || enabled {
			result[h.Name] = true
		}
	}
	return result
}
