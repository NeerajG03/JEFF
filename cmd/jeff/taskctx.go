package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF/workspace"
)

// resolveTaskID determines the task ID and task directory from args or cwd.
// If args has an ID, looks up the workspace. Otherwise auto-detects from cwd
// by walking up to find a task directory under JEFF_HOME/tasks/.
func resolveTaskID(args []string) (taskID string, taskDir string, err error) {
	if len(args) > 0 {
		td, err := workspace.Open(cfg.Home, args[0])
		if err != nil {
			// Task ID provided but no workspace — return ID without dir.
			// Some commands (like done) may not need the dir.
			return args[0], "", nil
		}
		return args[0], td.Path, nil
	}

	// Auto-detect from cwd: check if cwd is inside JEFF_HOME/tasks/<slug>/.
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("get cwd: %w", err)
	}

	// Resolve symlinks for reliable comparison.
	tasksDir, _ := filepath.EvalSymlinks(filepath.Join(cfg.Home, "tasks"))
	cwdResolved, _ := filepath.EvalSymlinks(cwd)

	// Check if cwd is under tasks/.
	if !strings.HasPrefix(cwdResolved, tasksDir+string(filepath.Separator)) && cwdResolved != tasksDir {
		return "", "", fmt.Errorf("not inside a task workspace — provide a task ID")
	}

	// Extract the task dir slug — first path component after tasks/.
	rel, _ := filepath.Rel(tasksDir, cwdResolved)
	slug := strings.SplitN(rel, string(filepath.Separator), 2)[0]

	taskID = workspace.ExtractTaskID(slug)
	if !strings.HasPrefix(taskID, "gig-") {
		return "", "", fmt.Errorf("not inside a task workspace — provide a task ID")
	}

	return taskID, filepath.Join(tasksDir, slug), nil
}
