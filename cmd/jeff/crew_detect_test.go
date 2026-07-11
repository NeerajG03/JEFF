package main

// Tests for gig-9c92 Option A/B: orchestrator identity detection resolution
// order and the widened session-name regex.

import "testing"

// depsWith builds orchestratorDetectDeps from simple maps/values so the
// resolution order can be exercised without a live tmux or crew DB.
func depsWith(env map[string]string, paneToOrch map[string]string, sessionName string) orchestratorDetectDeps {
	return orchestratorDetectDeps{
		getenv: func(k string) string { return env[k] },
		paneLookup: func(pane string) string {
			if paneToOrch == nil {
				return ""
			}
			return paneToOrch[pane]
		},
		sessionName: func(string) string { return sessionName },
	}
}

func TestDetectOrchestratorResolutionOrder(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		paneToOrch  map[string]string
		sessionName string
		want        string
	}{
		{
			name: "env var wins over everything",
			env:  map[string]string{"JEFF_ORCHESTRATOR_SESSION": "jeff-DM20", "TMUX": "1", "TMUX_PANE": "%42"},
			// Even a conflicting pane binding must not override the explicit env var.
			paneToOrch:  map[string]string{"%42": "jeff-other"},
			sessionName: "jeff-other",
			want:        "jeff-DM20",
		},
		{
			name:        "not inside tmux and no env var returns empty",
			env:         map[string]string{},
			sessionName: "jeff-DM20",
			want:        "",
		},
		{
			name:        "persisted pane binding used when no env var",
			env:         map[string]string{"TMUX": "1", "TMUX_PANE": "%42"},
			paneToOrch:  map[string]string{"%42": "jeff-DM20"},
			sessionName: "jeff", // regex would only give "jeff"; pane binding wins
			want:        "jeff-DM20",
		},
		{
			name:        "pane binding survives a renamed session",
			env:         map[string]string{"TMUX": "1", "TMUX_PANE": "%42"},
			paneToOrch:  map[string]string{"%42": "jeff-DM20"},
			sessionName: "some-unrelated-session",
			want:        "jeff-DM20",
		},
		{
			name:        "falls back to session-name regex when no binding",
			env:         map[string]string{"TMUX": "1", "TMUX_PANE": "%42"},
			paneToOrch:  map[string]string{}, // no binding
			sessionName: "jeff-work",
			want:        "jeff-work",
		},
		{
			name:        "bare jeff session now resolves (widened regex)",
			env:         map[string]string{"TMUX": "1", "TMUX_PANE": "%42"},
			paneToOrch:  map[string]string{},
			sessionName: "jeff",
			want:        "jeff",
		},
		{
			name:        "non-jeff session returns empty (signals fail-loud to caller)",
			env:         map[string]string{"TMUX": "1", "TMUX_PANE": "%42"},
			paneToOrch:  map[string]string{},
			sessionName: "randomshell",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectOrchestratorIDWith(depsWith(tt.env, tt.paneToOrch, tt.sessionName))
			if got != tt.want {
				t.Errorf("detectOrchestratorIDWith = %q, want %q", got, tt.want)
			}
		})
	}
}
