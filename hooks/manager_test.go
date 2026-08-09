package hooks

import (
	"os"
	"sort"
	"testing"
)

func testRegistry() *Registry {
	r := NewRegistry()
	r.Register(&Hook{
		Name:    "hook-a",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return "#!/bin/bash\necho hook-a"
			},
			"opencode": func(ctx HookContext) string {
				return `  // [hook-a]
  parts.push("a");`
			},
		},
	})
	r.Register(&Hook{
		Name:    "hook-b",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return "#!/bin/bash\necho hook-b"
			},
			"opencode": func(ctx HookContext) string {
				return `  // [hook-b]
  parts.push("b");`
			},
		},
	})
	r.Register(&Hook{
		Name:    "hook-c",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return "#!/bin/bash\necho hook-c"
			},
		},
	})
	return r
}

func TestSyncInstallsEnabled(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	enabled := map[string]bool{"hook-a": true, "hook-b": true}
	if err := mgr.Sync(dir, enabled, "claude", ctx); err != nil {
		t.Fatal(err)
	}

	installed := mgr.Installed(dir, "claude")
	sort.Strings(installed)
	if len(installed) != 2 || installed[0] != "hook-a" || installed[1] != "hook-b" {
		t.Fatalf("installed = %v, want [hook-a, hook-b]", installed)
	}

	// Check scripts exist.
	for _, name := range installed {
		if _, err := os.Stat(scriptPath(dir, name)); err != nil {
			t.Fatalf("script %s missing: %v", name, err)
		}
	}
}

func TestSyncUninstallsExtra(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	// Install both.
	enabled := map[string]bool{"hook-a": true, "hook-b": true}
	mgr.Sync(dir, enabled, "claude", ctx)

	// Sync with only hook-a.
	enabled = map[string]bool{"hook-a": true}
	if err := mgr.Sync(dir, enabled, "claude", ctx); err != nil {
		t.Fatal(err)
	}

	installed := mgr.Installed(dir, "claude")
	if len(installed) != 1 || installed[0] != "hook-a" {
		t.Fatalf("installed = %v, want [hook-a]", installed)
	}

	// hook-b script should be gone.
	if _, err := os.Stat(scriptPath(dir, "hook-b")); !os.IsNotExist(err) {
		t.Fatal("hook-b script should be removed")
	}
}

func TestSyncIdempotent(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	enabled := map[string]bool{"hook-a": true}
	mgr.Sync(dir, enabled, "claude", ctx)
	mgr.Sync(dir, enabled, "claude", ctx)

	installed := mgr.Installed(dir, "claude")
	if len(installed) != 1 {
		t.Fatalf("installed = %v after double sync, want [hook-a]", installed)
	}
}

func TestSyncOpenCodeAgent(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	enabled := map[string]bool{"hook-a": true, "hook-b": true}
	if err := mgr.Sync(dir, enabled, "opencode", ctx); err != nil {
		t.Fatal(err)
	}

	// Plugin file should exist.
	if _, err := os.Stat(openCodePluginPath(dir)); err != nil {
		t.Fatalf("opencode plugin not created: %v", err)
	}
}

func TestSyncPreservesUnknownHooks(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	// Manually create a hook script not in our registry.
	hdir := hooksDir(dir)
	os.MkdirAll(hdir, 0o755)
	os.WriteFile(scriptPath(dir, "external-hook"), []byte("#!/bin/bash"), 0o755)

	enabled := map[string]bool{"hook-a": true}
	mgr.Sync(dir, enabled, "claude", ctx)

	// External hook should still exist.
	if _, err := os.Stat(scriptPath(dir, "external-hook")); err != nil {
		t.Fatal("external hook should not be removed by Sync")
	}
}

// TestSyncRemovesOrphanedJeffManagedHook reproduces gig-1d9d.16 rule 3: a hook
// removed from the registry (e.g. the old inbox-check.sh poll) must actually
// disappear from a task dir on the next sync, not linger forever because
// Sync's uninstall pass only ever considered hooks still declared by the
// registry. TestSyncPreservesUnknownHooks (above) must keep passing alongside
// this — a script we never generated (no version marker) is never touched.
func TestSyncRemovesOrphanedJeffManagedHook(t *testing.T) {
	dir := t.TempDir()
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	// Simulate a hook that used to be in the registry: install it for real
	// (writes the script with jeff's version marker + a settings.json entry).
	// Then sync with a registry that no longer declares it at all — exactly
	// what removing a hook from builtinHooks() looks like on an existing dir.
	legacy := &Hook{
		Name:    "legacy-hook",
		Source:  SourceTask,
		Event:   "PostToolUse",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string { return "#!/bin/bash\necho legacy" },
		},
	}
	if err := installClaudeScript(legacy, dir, ctx, legacy.Scripts["claude"], claudeSettingsPath(dir)); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(testRegistry()) // does NOT contain legacy-hook
	enabled := map[string]bool{"hook-a": true}
	if err := mgr.Sync(dir, enabled, "claude", ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(scriptPath(dir, "legacy-hook")); !os.IsNotExist(err) {
		t.Error("legacy-hook script should be removed by Sync — the registry no longer declares it")
	}
	settings, err := readSettingsFile(claudeSettingsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	hooksMap, _ := settings["hooks"].(map[string]any)
	if postToolUse, ok := hooksMap["PostToolUse"]; ok {
		t.Errorf("legacy-hook settings.json entry should be removed by Sync: %v", postToolUse)
	}
}

func TestInstallNotFound(t *testing.T) {
	mgr := NewManager(NewRegistry())
	ctx := HookContext{}
	err := mgr.Install("nonexistent", t.TempDir(), "claude", ctx)
	if err == nil {
		t.Fatal("expected error for missing hook")
	}
}

func TestUninstallAll(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	enabled := map[string]bool{"hook-a": true, "hook-b": true, "hook-c": true}
	if err := mgr.Sync(dir, enabled, "claude", ctx); err != nil {
		t.Fatal(err)
	}

	installed := mgr.Installed(dir, "claude")
	if len(installed) != 3 {
		t.Fatalf("precondition: installed = %v, want 3 hooks", installed)
	}

	mgr.UninstallAll(dir)

	installed = mgr.Installed(dir, "claude")
	if len(installed) != 0 {
		t.Fatalf("after UninstallAll: installed = %v, want empty", installed)
	}

	for _, name := range []string{"hook-a", "hook-b", "hook-c"} {
		if _, err := os.Stat(scriptPath(dir, name)); !os.IsNotExist(err) {
			t.Errorf("script %s should be removed after UninstallAll", name)
		}
	}

	settings, err := readSettingsFile(settingsPath(dir))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if _, ok := settings["hooks"]; ok {
		t.Error("settings.json should have no hooks key after UninstallAll")
	}
}

func TestUninstallAllFromDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())
	ctx := HookContext{JeffHome: dir, TargetDir: dir}

	enabled := map[string]bool{"hook-a": true}
	mgr.Sync(dir, enabled, "claude", ctx)

	UninstallAllFromDir(dir)

	if len(mgr.Installed(dir, "claude")) != 0 {
		t.Error("UninstallAllFromDir should remove all hooks")
	}
}

func TestUninstallAllEmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(testRegistry())

	// Should not panic or error on an empty directory.
	mgr.UninstallAll(dir)
}
