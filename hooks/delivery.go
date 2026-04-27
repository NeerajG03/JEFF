package hooks

import "sync"

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
