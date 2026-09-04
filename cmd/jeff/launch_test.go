package main

import (
	"testing"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/spf13/cobra"
)

func boolPtr(b bool) *bool { return &b }

func TestEffectiveSkipPermissions(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *jeff.Config
		safeFlag bool
		want     bool
	}{
		{"default with no config and no flag", &jeff.Config{}, false, true},
		{"--safe overrides unset config", &jeff.Config{}, true, false},
		{"config true, no flag", &jeff.Config{SkipPermissions: boolPtr(true)}, false, true},
		{"config false, no flag", &jeff.Config{SkipPermissions: boolPtr(false)}, false, false},
		{"--safe overrides config true", &jeff.Config{SkipPermissions: boolPtr(true)}, true, false},
		{"--safe overrides config false (no-op, already false)", &jeff.Config{SkipPermissions: boolPtr(false)}, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveSkipPermissions(tc.cfg, tc.safeFlag)
			if got != tc.want {
				t.Errorf("effectiveSkipPermissions(cfg, %v) = %v, want %v", tc.safeFlag, got, tc.want)
			}
		})
	}
}

func TestAgentCompletion(t *testing.T) {
	comps, directive := agentCompletion(nil, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected directive NoFileComp (%v), got %v", cobra.ShellCompDirectiveNoFileComp, directive)
	}

	expectedAgents := []string{"claude", "gemini", "opencode", "codex"}
	for _, want := range expectedAgents {
		found := false
		for _, comp := range comps {
			if len(comp) >= len(want) && comp[:len(want)] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agentCompletion missing expected agent %q in completions: %v", want, comps)
		}
	}
}

func TestAgentFlagValidation(t *testing.T) {
	tests := []struct {
		input   string
		isValid bool
	}{
		{"claude", true},
		{"opencode", true},
		{"gemini", true},
		{"codex", true},
		{"invalid_agent", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := jeff.AgentTool(tc.input).IsValid()
			if got != tc.isValid {
				t.Errorf("AgentTool(%q).IsValid() = %v, want %v", tc.input, got, tc.isValid)
			}
		})
	}
}
