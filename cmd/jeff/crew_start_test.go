package main

import (
	jeff "github.com/NeerajG03/JEFF"
	"testing"
)

func TestCrewStartArgs(t *testing.T) {
	// Mock cfg to avoid panic
	cfg = &jeff.Config{Home: "/tmp/jeff-test"}
	tests := []struct {
		name      string
		args      []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "positional prompt provided",
			args:      []string{"gig-123", "fix the auth bug"},
			wantError: false,
		},
		{
			name:      "prompt flag provided (deprecated)",
			args:      []string{"gig-123", "--prompt", "fix the auth bug"},
			wantError: false,
		},
				{
			name:      "both positional AND flag provided (positional wins)",
			args:      []string{"gig-123", "positional value", "--prompt", "flag value"},
			wantError: false,
		},
		{
			name:      "both missing",
			args:      []string{"gig-123"},
			wantError: true,
			errMsg:    "missing required prompt. Usage: jeff crew start <gig-id> \"<prompt>\" [flags]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := crewStartCmd()
			
			// We only want to test the argument parsing layer up to the prompt validation.
			// Because actual RunE requires tmux/DB which will fail, we intercept it.
			// However, cobra's ExecuteC is the easiest way.
			
			// Mock RunE to do nothing, but keep our prompt parsing logic inside.
			// Wait, the prompt parsing is inside the original RunE!
			// If we run the original RunE, it will fail on workspace.Open.
			// So we can just test if the error is exactly "missing required prompt..."
			// If it fails with "workspace not found...", it means it passed the prompt check!
			
			cmd.SetArgs(tt.args)
			// don't execute normal print
			
			err := cmd.Execute()
			
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMsg)
				}
				if err.Error() != tt.errMsg {
					t.Fatalf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					// We expect it to pass the prompt check and fail on workspace.Open
					// "workspace not found for gig-123: ..."
					if err.Error() == "missing required prompt. Usage: jeff crew start <gig-id> \"<prompt>\" [flags]" {
						t.Fatalf("unexpected prompt error: %v", err)
					}
				}
			}
		})
	}
}
