package main

import (
	"testing"
)

func TestOrchestratorSessionRegex(t *testing.T) {
	re := orchestratorSessionRe

	match := []string{
		"jeff-1",
		"jeff-2",
		"jeff-10",
		"jeff-work",
		"jeff-work-2",
		"jeff-a",
		"jeff-abc",
		"jeff-abc-def",
	}
	noMatch := []string{
		"jeff",
		"jeff-",
		"jeff-Work",       // uppercase not allowed
		"jeff-WORK",       // uppercase not allowed
		"jeff_work",       // underscore not allowed
		"jeff-work_thing", // underscore not allowed
		"jeff--work",      // double dash: second char must be [a-z0-9]
		"notjeff-1",
		"",
		"jeff-1 ",         // trailing space
		" jeff-1",         // leading space
	}

	for _, s := range match {
		if !re.MatchString(s) {
			t.Errorf("expected %q to match orchestrator session regex, but it did not", s)
		}
	}
	for _, s := range noMatch {
		if re.MatchString(s) {
			t.Errorf("expected %q NOT to match orchestrator session regex, but it did", s)
		}
	}
}

func TestDetectOrchestratorID_EnvVarPriority(t *testing.T) {
	// When JEFF_ORCHESTRATOR_SESSION is set, it must be returned regardless of TMUX.
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "jeff-42")
	t.Setenv("TMUX", "")

	got := detectOrchestratorID()
	if got != "jeff-42" {
		t.Errorf("detectOrchestratorID() = %q, want %q", got, "jeff-42")
	}
}

func TestDetectOrchestratorID_EnvVarOverridesTmux(t *testing.T) {
	// Env var wins even when inside a tmux session.
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "jeff-env")
	t.Setenv("TMUX", "/tmp/tmux-1234/default,1234,0")
	// TMUX_PANE left unset — if tmux branch ran it would try to exec tmux,
	// but env var check is first so tmux is never called.

	got := detectOrchestratorID()
	if got != "jeff-env" {
		t.Errorf("detectOrchestratorID() = %q, want %q (env var should win)", got, "jeff-env")
	}
}

func TestDetectOrchestratorID_NoEnvNoTmux(t *testing.T) {
	// Neither env var nor tmux — should return "".
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "")
	t.Setenv("TMUX", "")

	got := detectOrchestratorID()
	if got != "" {
		t.Errorf("detectOrchestratorID() = %q, want %q (no tmux, no env)", got, "")
	}
}

func TestDetectOrchestratorID_EmptyEnvFallsThrough(t *testing.T) {
	// Empty env var must not be treated as a valid ID.
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "")
	t.Setenv("TMUX", "")

	got := detectOrchestratorID()
	if got != "" {
		t.Errorf("detectOrchestratorID() = %q, want empty string", got)
	}
}

func TestResolveCrewListOrchestratorFilter(t *testing.T) {
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "jeff-42")
	t.Setenv("TMUX", "")

	// --all bypasses orchestrator filter regardless of env var.
	if got := resolveCrewListOrchestratorFilter(true, ""); got != "" {
		t.Errorf("showAll=true: got %q, want empty (all orchestrators)", got)
	}

	// --orchestrator flag wins over auto-detect.
	if got := resolveCrewListOrchestratorFilter(false, "jeff-99"); got != "jeff-99" {
		t.Errorf("--orchestrator flag: got %q, want jeff-99", got)
	}

	// No flag: env var is used.
	if got := resolveCrewListOrchestratorFilter(false, ""); got != "jeff-42" {
		t.Errorf("env var auto-detect: got %q, want jeff-42", got)
	}

	// No flag, no env → empty (outside any orchestrator).
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "")
	if got := resolveCrewListOrchestratorFilter(false, ""); got != "" {
		t.Errorf("no orchestrator context: got %q, want empty", got)
	}

	// --all takes precedence even when --orchestrator is also set.
	t.Setenv("JEFF_ORCHESTRATOR_SESSION", "jeff-42")
	if got := resolveCrewListOrchestratorFilter(true, "jeff-55"); got != "" {
		t.Errorf("--all with --orchestrator flag: got %q, want empty (--all wins)", got)
	}
}
