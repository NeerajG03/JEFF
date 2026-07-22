package main

import (
	"testing"

	jeff "github.com/NeerajG03/JEFF"
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
