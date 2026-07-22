package persona_test

import (
	"testing"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/persona"
)

func TestDefaultAgentIsValid(t *testing.T) {
	for _, name := range persona.Names() {
		agent := persona.DefaultAgent(name)
		if agent == "" {
			continue // Checked by TestPersonaConsistency
		}
		if !jeff.AgentTool(agent).IsValid() {
			t.Errorf("persona %q has invalid default agent: %q", name, agent)
		}
	}
}
