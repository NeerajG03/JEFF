package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func openCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open [gig-id]",
		Short: "Open a task workspace (or JEFF_HOME) in your configured IDE",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ide := cfg.IDE
			if ide == "" {
				ide = jeff.IDEVSCode
			}

			// Case 1: explicit task ID.
			if len(args) == 1 {
				return openTask(args[0], ide)
			}

			// Case 2: auto-detect from cwd.
			taskID, taskDir, err := resolveTaskID(nil)
			if err == nil && taskID != "" && taskDir != "" {
				fmt.Fprintf(os.Stderr, "Opening %s in %s\n", taskID, ide)
				return openIDE(taskDir, ide)
			}

			// Case 3: not in a task dir, no args — open JEFF_HOME.
			fmt.Fprintf(os.Stderr, "Opening JEFF_HOME in %s\n", ide)
			return openIDE(cfg.Home, ide)
		},
	}
	cmd.ValidArgsFunction = openableTaskCompletion
	return cmd
}

// openTask opens a specific task's workspace after validating it exists and is in_progress.
func openTask(taskID string, ide jeff.IDE) error {
	// Check workspace exists.
	td, err := workspace.Open(cfg.Home, taskID)
	if err != nil {
		return fmt.Errorf("no workspace for %s: %w", taskID, err)
	}

	// Check gig status — must be in_progress (or open, for tasks not yet claimed).
	store, err := openGigStore(cfg)
	if err == nil {
		defer store.Close()
		task, err := store.Get(taskID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not verify task status: %v\n", err)
		} else if task.Status.IsTerminal() {
			return fmt.Errorf("task %s is %s — cannot open a closed/cancelled task", taskID, task.Status)
		}
	}

	fmt.Fprintf(os.Stderr, "Opening %s in %s\n", taskID, ide)
	return openIDE(td.Path, ide)
}

// openIDE launches the configured IDE at the given directory.
func openIDE(dir string, ide jeff.IDE) error {
	bin, err := exec.LookPath(ide.Command())
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", ide.Command(), err)
	}

	var cmd *exec.Cmd
	if ide.Terminal() {
		// Terminal-based editors (e.g. nvim) use cwd for their root.
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

	fmt.Fprintf(os.Stderr, "Opened %s in %s\n", dir, ide)
	return nil
}

// openableTaskCompletion completes tasks that have workspaces and are in_progress.
func openableTaskCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dirs, err := workspace.ListActive(cfg.Home, gigTaskPrefix(cfg))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	store, _ := openGigStore(cfg)
	if store != nil {
		defer store.Close()
	}

	var ids []string
	for _, td := range dirs {
		// Only include tasks that are in_progress (have a workspace + active in gig).
		if store != nil {
			t, err := store.Get(td.TaskID)
			if err != nil {
				continue
			}
			if t.Status != gig.StatusInProgress {
				continue
			}
			ids = append(ids, td.TaskID+"\t"+t.Title)
		} else {
			ids = append(ids, td.TaskID+"\t"+td.Slug)
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}
