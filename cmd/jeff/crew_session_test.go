package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NeerajG03/JEFF/identity"
)

// The exhaustive resolution-chain coverage lives in the identity package
// (hermetic, injected inputs). These tests cover the cmd-layer wrappers.

// TestDetectOrchestratorID_EnvOverride confirms the env override is honored by
// the thin wrapper (both the primary and legacy variable names).
func TestDetectOrchestratorID_EnvOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(identity.EnvVar, "jeff-42")

	id, src, err := detectOrchestratorID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "jeff-42" || src != identity.SourceEnv {
		t.Errorf("got (%q, %q), want (jeff-42, env)", id, src)
	}
}

func TestDetectOrchestratorID_LegacyEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(identity.EnvVar, "")
	t.Setenv(identity.EnvVarLegacy, "jeff-env")

	id, _, err := detectOrchestratorID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "jeff-env" {
		t.Errorf("got %q, want jeff-env (legacy env alias)", id)
	}
}

// TestResolveCrewListOrchestratorFilter covers the --all / --orchestrator / env
// precedence of the crew list filter.
func TestResolveCrewListOrchestratorFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(identity.EnvVarLegacy, "")
	t.Setenv(identity.EnvVar, "jeff-42")

	// --all bypasses the orchestrator filter regardless of env var.
	if got, err := resolveCrewListOrchestratorFilter(true, ""); err != nil || got != "" {
		t.Errorf("showAll=true: got (%q, %v), want (\"\", nil)", got, err)
	}

	// --orchestrator flag wins over auto-detect.
	if got, err := resolveCrewListOrchestratorFilter(false, "jeff-99"); err != nil || got != "jeff-99" {
		t.Errorf("--orchestrator flag: got (%q, %v), want (jeff-99, nil)", got, err)
	}

	// No flag: identity env var is used.
	if got, err := resolveCrewListOrchestratorFilter(false, ""); err != nil || got != "jeff-42" {
		t.Errorf("env auto-detect: got (%q, %v), want (jeff-42, nil)", got, err)
	}
}

// TestResolveCrewListOrchestratorFilter_NoIdentity confirms an absent identity
// resolves to an empty filter (not an error), so `crew list` shows the
// unscoped active set rather than failing.
func TestResolveCrewListOrchestratorFilter_NoIdentity(t *testing.T) {
	// A home with no ancestor relation to cwd + no files → clean not-found.
	t.Setenv("HOME", t.TempDir())
	t.Setenv(identity.EnvVar, "")
	t.Setenv(identity.EnvVarLegacy, "")

	got, err := resolveCrewListOrchestratorFilter(false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (no identity configured)", got)
	}
}

// TestResolveCrewListOrchestratorFilter_MalformedFailsLoud confirms a corrupt
// per-project identity file makes the filter resolution fail loud rather than
// silently degrade.
func TestResolveCrewListOrchestratorFilter_MalformedFailsLoud(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(identity.EnvVar, "")
	t.Setenv(identity.EnvVarLegacy, "")
	// Run from a directory holding a malformed identity file.
	path := identity.ProjectFilePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, dir)
	defer restore()

	if _, err := resolveCrewListOrchestratorFilter(false, ""); err == nil {
		t.Fatal("expected error on malformed identity file, got nil")
	}
}

// chdir switches the working directory for a test and returns a restore func.
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(prev) }
}
