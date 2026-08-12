package hooks

import (
	"sort"
	"sync"
)

// Delivery abstracts agent-specific hook installation.
// Each agent CLI has its own mechanism for running hooks
// (Claude uses bash scripts + settings.json, OpenCode uses a JS plugin, etc.).
type Delivery interface {
	// ScriptKey returns the key used in Hook.Scripts to find this delivery's script generator.
	ScriptKey() string

	// Install writes a single hook's artifacts to targetDir.
	Install(h *Hook, targetDir string, ctx HookContext) error

	// Uninstall removes a single hook's artifacts from targetDir.
	Uninstall(name, targetDir string) error

	// SyncAll is called after individual installs/uninstalls to do batch operations.
	// For Claude this is a no-op; for OpenCode it regenerates the combined JS plugin.
	SyncAll(enabled []*Hook, targetDir string, ctx HookContext) error

	// Installed returns names of hooks currently installed at targetDir.
	Installed(targetDir string) []string

	// IsManaged reports whether the installed artifact for name was generated
	// by jeff (carries its version marker) as opposed to authored externally.
	// Sync uses this to decide whether an installed hook the registry no
	// longer declares at all is safe to remove: a hook jeff once generated and
	// later dropped from code (orphaned) vs. a user's own hook that happens to
	// share a name (must never be touched). Presence-only, not content-aware —
	// see scriptHasVersionMarker's doc comment for the known limitation and
	// why it fails safe.
	IsManaged(targetDir, name string) bool

	// EventName maps a Hook to the name this delivery's own event system uses —
	// e.g. Gemini's PostToolUse -> AfterTool (geminiEventName), or OpenCode's
	// per-hook OpenCodeEvent override. A delivery that reuses Claude's event
	// names unchanged returns h.Event verbatim.
	//
	// This exists so a script shared across deliveries (bashBoth) can be
	// checked generically: any hookSpecificOutput.hookEventName it embeds must
	// equal ITS delivery's EventName(h), not h.Event literally — a
	// hardcoded literal is valid JSON but the wrong contract under any
	// delivery whose mapping differs from Claude's (#106 follow-up).
	EventName(h *Hook) string
}

var (
	deliveriesMu sync.RWMutex
	deliveries   = make(map[string]Delivery)
)

// RegisterDelivery registers a Delivery by its ScriptKey. Panics on duplicate.
func RegisterDelivery(d Delivery) {
	deliveriesMu.Lock()
	defer deliveriesMu.Unlock()
	if _, exists := deliveries[d.ScriptKey()]; exists {
		panic("hooks: duplicate delivery: " + d.ScriptKey())
	}
	deliveries[d.ScriptKey()] = d
}

// GetDelivery returns the Delivery for the given key, or nil if not registered.
func GetDelivery(key string) Delivery {
	deliveriesMu.RLock()
	defer deliveriesMu.RUnlock()
	return deliveries[key]
}

// DeliveryKeys returns the ScriptKeys of every registered delivery, sorted.
// Code that must cover every delivery (e.g. a contract test) should iterate
// this instead of a hardcoded list of names, so a newly registered delivery
// is covered the day it registers rather than the day someone remembers to
// update a switch statement.
func DeliveryKeys() []string {
	deliveriesMu.RLock()
	defer deliveriesMu.RUnlock()
	keys := make([]string, 0, len(deliveries))
	for k := range deliveries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
