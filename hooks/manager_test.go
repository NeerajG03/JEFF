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

	installed := mgr.Installed(dir)
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

	installed := mgr.Installed(dir)
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

	installed := mgr.Installed(dir)
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

func TestInstallNotFound(t *testing.T) {
	mgr := NewManager(NewRegistry())
	ctx := HookContext{}
	err := mgr.Install("nonexistent", t.TempDir(), "claude", ctx)
	if err == nil {
		t.Fatal("expected error for missing hook")
	}
}
