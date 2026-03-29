package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func openCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open [gig-id]",
		Short: "Open JEFF_HOME or a task workspace in your configured IDE",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := cfg.Home
			if len(args) == 1 {
				td, err := workspace.Open(cfg.Home, args[0])
				if err != nil {
					return fmt.Errorf("task %s: %w", args[0], err)
				}
				dir = td.Path
			}

			ide := cfg.IDE
			if ide == "" {
				ide = jeff.IDEVSCode
			}

			return openIDE(dir, ide)
		},
	}
	cmd.ValidArgsFunction = activeTaskCompletion
	return cmd
}

// openIDE launches the configured IDE at the given directory.
func openIDE(dir string, ide jeff.IDE) error {
	bin, err := exec.LookPath(ide.Command())
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", ide.Command(), err)
	}

	var cmd *exec.Cmd
	if ide.Terminal() {
		// Terminal-based editors (e.g. nvim) use cwd for their root,
		// so we cd into the target dir and open ".".
		cmd = exec.Command(bin, ".")
		cmd.Dir = dir
	} else {
		cmd = exec.Command(bin, dir)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if ide.Terminal() {
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("open %s: %w", ide, err)
		}
		return nil
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", ide, err)
	}

	fmt.Printf("Opened %s in %s\n", dir, ide)
	return nil
}
