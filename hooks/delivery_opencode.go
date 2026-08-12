package hooks

import "fmt"

// opencodeDelivery generates a combined JS plugin from enabled hooks.
type opencodeDelivery struct{}

func init() {
	RegisterDelivery(&opencodeDelivery{})
}

func (d *opencodeDelivery) ScriptKey() string { return "opencode" }

func (d *opencodeDelivery) Install(h *Hook, targetDir string, ctx HookContext) error {
	return nil // OpenCode batches everything in SyncAll.
}

func (d *opencodeDelivery) Uninstall(name, targetDir string) error {
	return nil // OpenCode batches everything in SyncAll.
}

func (d *opencodeDelivery) SyncAll(enabled []*Hook, targetDir string, ctx HookContext) error {
	// Filter to hooks that have an opencode script generator.
	var ocHooks []*Hook
	for _, h := range enabled {
		if h.Scripts[d.ScriptKey()] != nil {
			ocHooks = append(ocHooks, h)
		}
	}
	if err := syncOpenCode(ocHooks, targetDir, ctx); err != nil {
		return fmt.Errorf("sync opencode plugin: %w", err)
	}
	return nil
}

func (d *opencodeDelivery) Installed(targetDir string) []string {
	// OpenCode doesn't have individual hook files — return nil.
	return nil
}

func (d *opencodeDelivery) IsManaged(targetDir, name string) bool {
	// Never consulted: Installed always returns nil for OpenCode, since the
	// combined plugin is fully regenerated from enabled hooks on every sync.
	return false
}

// EventName returns the mapped event name for a hook under OpenCode,
// respecting any per-hook Hook.OpenCodeEvent override (e.g. session.idle for worker-stop).
func (d *opencodeDelivery) EventName(h *Hook) string {
	if h == nil {
		return ""
	}
	return openCodeEventName(h)
}
