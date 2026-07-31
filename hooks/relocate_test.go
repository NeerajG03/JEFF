package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAddHookToSettingsRefreshesStalePath is the regression test for the
// unfixable-hook-path half of #84.
//
// Dedup is by script basename, so an entry whose command still names a previous
// JEFF home matched and short-circuited addHookToSettings. The stale absolute path
// then survived every `jeff init --update` and every hook sync — the settings could
// never be repaired, only hand-edited.
func TestAddHookToSettingsRefreshesStalePath(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/old/home/.jeff/hooks/jeff-instructions.sh",
							"timeout": 5,
						},
					},
				},
			},
		},
	}

	const want = "/new/place/jeff/hooks/jeff-instructions.sh"
	addHookToSettings(settings, "SessionStart", "", want, 9)

	blocks := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("block count = %d, want 1 (the entry must be updated, not duplicated)", len(blocks))
	}
	entry := blocks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if got := entry["command"].(string); got != want {
		t.Errorf("command = %q, want %q — a stale hook path must be corrected on sync", got, want)
	}
	// timeout is deliberately NOT touched — it may have been customized by hand,
	// and the old dedup path never updated it either.
	if got := entry["timeout"]; got != 5 {
		t.Errorf("timeout = %v, want 5 (preserved, not overwritten)", got)
	}
}

// TestAddHookToSettingsIsIdempotent confirms the refresh does not churn a
// correct entry or duplicate it.
func TestAddHookToSettingsIsIdempotent(t *testing.T) {
	const path = "/home/.jeff/hooks/jeff-repos.sh"
	settings := map[string]any{}

	addHookToSettings(settings, "SessionStart", "", path, 5)
	addHookToSettings(settings, "SessionStart", "", path, 5)

	blocks := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("block count = %d, want 1 after two identical installs", len(blocks))
	}
	entry := blocks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if got := entry["command"].(string); got != path {
		t.Errorf("command = %q, want %q", got, path)
	}
}

// TestRefreshHookCommandPreservesSurroundingArgv covers a command with extra argv:
// only the script token is rewritten.
func TestRefreshHookCommandPreservesSurroundingArgv(t *testing.T) {
	blocks := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "/bin/bash /old/home/hooks/crew-context.sh --flag",
					"timeout": 5,
				},
			},
		},
	}

	refreshHookCommand(blocks, "crew-context.sh", "/new/home/hooks/crew-context.sh")

	entry := blocks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	const want = "/bin/bash /new/home/hooks/crew-context.sh --flag"
	if got := entry["command"].(string); got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

// TestRefreshLeavesForeignHookAlone is the regression test for a finding from
// review: because dedup matches on script BASENAME alone, a user's own hook can
// share a name with one of ours. The first version of refreshHookCommand rewrote
// it — silently mutating hand-edited settings.json, strictly worse than the old
// skip. Repair is now limited to references that are jeff-shaped AND dangling.
func TestRefreshLeavesForeignHookAlone(t *testing.T) {
	// A live hook the user wrote, in their own directory, colliding on basename.
	live := filepath.Join(t.TempDir(), "jeff-instructions.sh")
	if err := os.WriteFile(live, []byte("#!/bin/bash\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": live, "timeout": 30},
					},
				},
			},
		},
	}

	addHookToSettings(settings, "SessionStart", "", "/home/.jeff/hooks/jeff-instructions.sh", 5)

	entry := settings["hooks"].(map[string]any)["SessionStart"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if got := entry["command"].(string); got != live {
		t.Errorf("clobbered a user's own hook: command = %q, want %q", got, live)
	}
	if got := entry["timeout"]; got != 30 {
		t.Errorf("clobbered a user's timeout: got %v, want 30", got)
	}
}

// TestRefreshLeavesLiveJeffHookAlone: even a jeff-shaped path is left alone while
// it still exists on disk. Only dangling references are repaired.
func TestRefreshLeavesLiveJeffHookAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, "worker-stop.sh")
	if err := os.WriteFile(live, []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	blocks := []any{
		map[string]any{"hooks": []any{map[string]any{"type": "command", "command": live, "timeout": 5}}},
	}
	refreshHookCommand(blocks, "worker-stop.sh", "/somewhere/else/hooks/worker-stop.sh")

	entry := blocks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if got := entry["command"].(string); got != live {
		t.Errorf("rewrote a live jeff hook: got %q, want %q", got, live)
	}
}

// TestRefreshIgnoresNonHooksDirLayout: a dangling path outside jeff's
// <dir>/hooks/<name>.sh layout is not ours to repair.
func TestRefreshIgnoresNonHooksDirLayout(t *testing.T) {
	blocks := []any{
		map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": "/gone/bin/worker-stop.sh", "timeout": 5,
		}}},
	}
	refreshHookCommand(blocks, "worker-stop.sh", "/home/.jeff/hooks/worker-stop.sh")

	entry := blocks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if got := entry["command"].(string); got != "/gone/bin/worker-stop.sh" {
		t.Errorf("repaired a path outside jeff's layout: got %q", got)
	}
}
