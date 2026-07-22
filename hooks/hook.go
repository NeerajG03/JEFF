// Package hooks provides a hook management system for injecting context
// into agent sessions (Claude Code, OpenCode, Gemini CLI).
package hooks

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ScriptVersion is the current version stamp written into generated hook scripts.
const ScriptVersion = "2"

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
	Event   string // agent event: "SessionStart", "PreCompact", "PostToolUse", "Stop"
	Matcher string // event matcher: "*", "Bash"
	Timeout int    // seconds; 0 defaults to 10
	// OpenCodeEvent optionally overrides the event mapping for OpenCode.
	OpenCodeEvent string

	// Scripts maps delivery keys to script generators.
	// Key = Delivery.ScriptKey() (e.g. "claude", "opencode", "gemini").
	Scripts map[string]func(ctx HookContext) string
}

// HookContext provides data that hook generators need.
type HookContext struct {
	JeffHome           string   // JEFF_HOME path
	TargetDir          string   // directory where hooks are being installed
	GigHome            string   // gig home for SDK calls (empty = default)
	TaskID             string   // gig task ID (set for task-level hooks)
	OrchestratorID     string   // orchestrator ID (set for orchestrator hooks)
	CheckpointPatterns []string // regex patterns that trigger checkpoint nudge
	Persona            string   // worker persona name (e.g. "jenko", "marlowe")
	Repos              []string // repos in scope for the task
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

// shellQuote wraps s in single quotes, escaping any single quotes within.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// TaskHooksStale reads any one generated script under <dir>/hooks/*.sh and
// returns true if the marker is missing or ≠ ScriptVersion (missing dir → false).
func TaskHooksStale(dir string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, "hooks", "*.sh"))
	if err != nil || len(matches) == 0 {
		return false
	}
	f, err := os.Open(matches[0])
	if err != nil {
		return true // treat unreadable as stale
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 3 && scanner.Scan(); i++ {
		line := scanner.Text()
		if strings.HasPrefix(line, "# jeff-hook-version: ") {
			v := strings.TrimPrefix(line, "# jeff-hook-version: ")
			return v != ScriptVersion
		}
	}
	return true
}
