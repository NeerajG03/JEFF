package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF"
)

// launchAgent starts the configured agent tool in the given directory.
// Uses the provider to determine the command and args.
func launchAgent(dir string, agent jeff.AgentTool, model string) error {
	p := jeff.GetProvider(agent)
	if p == nil {
		return fmt.Errorf("no provider registered for agent %q", agent)
	}

	bin, err := exec.LookPath(p.Command())
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", p.Command(), err)
	}

	args := p.BuildLaunchArgs(jeff.LaunchOpts{Model: model})

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited: %w", p.Command(), err)
	}
	return nil
}
