package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/memory"
)

// effectiveSkipPermissions resolves the launch permission mode:
// --safe flag > jeff.json skip_permissions > default (true, current behavior).
func effectiveSkipPermissions(cfg *jeff.Config, safeFlag bool) bool {
	if safeFlag {
		return false
	}
	if cfg.SkipPermissions != nil {
		return *cfg.SkipPermissions
	}
	return true
}

// launchAgent starts the configured agent tool in the given directory.
// Uses the provider to determine the command and args.
func launchAgent(dir string, agent jeff.AgentTool, model, agentName string, skip bool) error {
	p := jeff.GetProvider(agent)
	if p == nil {
		return fmt.Errorf("no provider registered for agent %q", agent)
	}

	bin, err := exec.LookPath(p.Command())
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", p.Command(), err)
	}

	args := p.BuildLaunchArgs(jeff.LaunchOpts{Model: model, AgentName: agentName, SkipPermissions: skip})

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range memory.EnvOverrides(agentName, string(agent)) {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited: %w", p.Command(), err)
	}
	return nil
}
