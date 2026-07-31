package hooks

import (
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
	if got := entry["timeout"]; got != 9 {
		t.Errorf("timeout = %v, want 9 (refreshed alongside the path)", got)
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

	refreshHookCommand(blocks, "crew-context.sh", "/new/home/hooks/crew-context.sh", 7)

	entry := blocks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	const want = "/bin/bash /new/home/hooks/crew-context.sh --flag"
	if got := entry["command"].(string); got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}
