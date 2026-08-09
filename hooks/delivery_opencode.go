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

// EventName uses the standard mapping only (openCodeStandardEventName),
// ignoring a hook's OpenCodeEvent override — this interface method takes a
// bare event string, not a *Hook, and OpenCode's contract test callers skip
// this delivery anyway (its scripts are injected plugin code, not stdin-piped
// bash), so this exists to satisfy Delivery, not for exact per-hook accuracy.
func (d *opencodeDelivery) EventName(claudeEvent string) string {
	return openCodeStandardEventName(claudeEvent)
}
