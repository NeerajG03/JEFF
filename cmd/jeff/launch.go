package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF"
)

// launchAgent starts the configured agent tool in the given directory.
// If model is non-empty and the agent is Claude Code, --model is appended.
func launchAgent(dir string, agent jeff.AgentTool, model string) error {
	bin, err := exec.LookPath(agent.Command())
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", agent.Command(), err)
	}

	args := agentArgs(agent, model)

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
// If model is non-empty and the agent supports it, --model is appended.
func agentArgs(agent jeff.AgentTool, model string) []string {
	switch agent {
	case jeff.AgentClaudeCode:
		args := []string{"--dangerously-skip-permissions"}
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	case jeff.AgentOpenCode:
		return nil
	default:
		return nil
	}
}
