package main

import (
	"testing"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
)

func TestBuildResumeOpts(t *testing.T) {
	// This test mocks the logic inside crewResumeCmd.
	// Since the logic is now verified to use agentTool and model correctly,
	// we just need to ensure the priority is right.

	cfg := &jeff.Config{
		Agent: "claude",
	}

	// personaName := "jenko"
	personaModel := "opus"

	tests := []struct {
		name          string
		existingAgent string
		existingModel string
		nilSession    bool
		wantAgent     string
		wantModel     string
	}{
		{
			name:          "Full override from DB",
			existingAgent: "gemini",
			existingModel: "flash",
			wantAgent:     "gemini",
			wantModel:     "flash",
		},
		{
			name:          "Legacy fallback (empty DB fields)",
			existingAgent: "",
			existingModel: "",
			wantAgent:     "claude",
			wantModel:     "opus",
		},
		{
			name:          "Partial override (agent only)",
			existingAgent: "opencode",
			existingModel: "",
			wantAgent:     "opencode",
			wantModel:     "opus",
		},
		{
			name:          "Partial override (model only)",
			existingAgent: "",
			existingModel: "haiku",
			wantAgent:     "claude",
			wantModel:     "haiku",
		},
		{
			name:       "Nil session fallback",
			nilSession: true,
			wantAgent:  "claude",
			wantModel:  "opus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Logic from crew_cmd.go:
			agentTool := cfg.Agent
			model := personaModel // simulated persona.RegisteredModel(cfg.Home, personaName)

			var existing *crew.Session
			if !tt.nilSession {
				existing = &crew.Session{
					Agent: tt.existingAgent,
					Model: tt.existingModel,
				}
			}
			agentTool, model = resolveResumeAgentModel(existing, agentTool, model)

			if string(agentTool) != tt.wantAgent {
				t.Errorf("agentTool = %q, want %q", agentTool, tt.wantAgent)
			}
			if model != tt.wantModel {
				t.Errorf("model = %q, want %q", model, tt.wantModel)
			}
		})
	}
}
