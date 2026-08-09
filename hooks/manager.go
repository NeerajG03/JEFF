package hooks

import "fmt"

// Manager orchestrates hook installation and removal.
type Manager struct {
	registry *Registry
}

// NewManager creates a manager backed by the given registry.
func NewManager(r *Registry) *Manager {
	return &Manager{registry: r}
}

// Install writes a single hook's artifacts to targetDir using the given delivery.
func (m *Manager) Install(name, targetDir, deliveryKey string, ctx HookContext) error {
	h := m.registry.Get(name)
	if h == nil {
		return fmt.Errorf("hook %q not found", name)
	}

	d := GetDelivery(deliveryKey)
	if d == nil {
		return fmt.Errorf("delivery %q not found", deliveryKey)
	}

	return d.Install(h, targetDir, ctx)
}

// Uninstall removes a single hook's artifacts from targetDir.
func (m *Manager) Uninstall(name, targetDir, deliveryKey string) error {
	d := GetDelivery(deliveryKey)
	if d == nil {
		return fmt.Errorf("delivery %q not found", deliveryKey)
	}
	return d.Uninstall(name, targetDir)
}

// Sync ensures targetDir has exactly the hooks in `enabled`.
// Installs missing hooks, uninstalls extra ones. Idempotent.
func (m *Manager) Sync(targetDir string, enabled map[string]bool, deliveryKey string, ctx HookContext) error {
	d := GetDelivery(deliveryKey)
	if d == nil {
		return fmt.Errorf("delivery %q not found", deliveryKey)
	}

	// Current state.
	installed := make(map[string]bool)
	for _, name := range d.Installed(targetDir) {
		installed[name] = true
	}

	// Install or update all enabled hooks (always overwrite scripts
	// so content updates propagate on sync).
	for name := range enabled {
		if err := d.Install(m.registry.Get(name), targetDir, ctx); err != nil {
			return fmt.Errorf("install hook %s: %w", name, err)
		}
	}

	// Uninstall extra hooks: ones still known to the registry but disabled,
	// and orphans — installed artifacts the registry has dropped entirely
	// (e.g. a hook deleted from builtinHooks()). An orphan is only removed
	// when jeff generated it (IsManaged); a genuinely external hook that
	// happens to share a name is never touched.
	for name := range installed {
		if enabled[name] {
			continue
		}
		known := m.registry.Get(name) != nil
		if !known && !d.IsManaged(targetDir, name) {
			continue
		}
		if err := d.Uninstall(name, targetDir); err != nil {
			return fmt.Errorf("uninstall hook %s: %w", name, err)
		}
	}

	// Run batch operation (e.g. OpenCode plugin regeneration).
	var enabledHooks []*Hook
	for name := range enabled {
		if h := m.registry.Get(name); h != nil {
			enabledHooks = append(enabledHooks, h)
		}
	}
	if err := d.SyncAll(enabledHooks, targetDir, ctx); err != nil {
		return fmt.Errorf("sync delivery %s: %w", deliveryKey, err)
	}

	return nil
}

// Installed returns names of hooks currently installed at targetDir
// using the specified delivery mechanism.
func (m *Manager) Installed(targetDir, deliveryKey string) []string {
	d := GetDelivery(deliveryKey)
	if d == nil {
		return nil
	}
	return d.Installed(targetDir)
}

// UninstallAll removes every installed hook from targetDir across all
// registered delivery mechanisms. Best-effort: individual failures are
// silently ignored so remaining hooks are still cleaned up.
func (m *Manager) UninstallAll(targetDir string) {
	deliveriesMu.RLock()
	keys := make([]string, 0, len(deliveries))
	for k := range deliveries {
		keys = append(keys, k)
	}
	deliveriesMu.RUnlock()

	// Collect all installed hook names before removing any. Deliveries
	// share the hooks/ directory for script files, so one delivery's
	// Uninstall can remove scripts that another delivery's Installed
	// would otherwise still see.
	names := make(map[string]bool)
	for _, key := range keys {
		for _, name := range m.Installed(targetDir, key) {
			names[name] = true
		}
	}

	for _, key := range keys {
		for name := range names {
			_ = m.Uninstall(name, targetDir, key)
		}
	}
}

// UninstallAllFromDir removes all installed hook artifacts from targetDir.
// Convenience wrapper for callers that don't need a custom registry.
func UninstallAllFromDir(targetDir string) {
	NewManager(DefaultRegistry()).UninstallAll(targetDir)
}
