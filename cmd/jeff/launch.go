package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF"
)

// launchAgent starts the configured agent tool in the given directory.
func launchAgent(dir string, agent jeff.AgentTool) error {
	bin, err := exec.LookPath(agent.Command())
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", agent.Command(), err)
	}

	args := agentArgs(agent)

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited: %w", agent.Command(), err)
	}
	return nil
}

// agentArgs returns default CLI arguments for each agent tool.
func agentArgs(agent jeff.AgentTool) []string {
	switch agent {
	case jeff.AgentClaudeCode:
		return []string{"--dangerously-skip-permissions"}
	case jeff.AgentOpenCode:
		return nil
	default:
		return nil
	}
}
