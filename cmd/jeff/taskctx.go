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

	sep := string(filepath.Separator)
	tasksDir := filepath.Join(cfg.Home, "tasks")

	// Find the tasks dir and cwd path to use for slug extraction.
	// Check the logical (unresolved) path first — this handles the case where cwd
	// is inside a worktree accessed via a symlink in the task dir (e.g. tasks/<slug>/repo/).
	// os.Getwd honours $PWD which gives the logical path through the symlink.
	// Only fall back to symlink-resolved paths for macOS /var→/private/var style indirection.
	var matchTasksDir, matchCwd string
	switch {
	case strings.HasPrefix(cwd, tasksDir+sep) || cwd == tasksDir:
		matchTasksDir, matchCwd = tasksDir, cwd
	default:
		// Fall back to fully-resolved paths.
		tasksDirResolved, _ := filepath.EvalSymlinks(tasksDir)
		cwdResolved, _ := filepath.EvalSymlinks(cwd)
		if strings.HasPrefix(cwdResolved, tasksDirResolved+sep) || cwdResolved == tasksDirResolved {
			matchTasksDir, matchCwd = tasksDirResolved, cwdResolved
		}
	}

	if matchTasksDir == "" {
		return "", "", fmt.Errorf("not inside a task workspace — provide a task ID")
	}

	// Extract the task dir slug — first path component after tasks/.
	rel, _ := filepath.Rel(matchTasksDir, matchCwd)
	slug := strings.SplitN(rel, sep, 2)[0]

	taskID = workspace.ExtractTaskID(slug)
	if !strings.HasPrefix(taskID, "gig-") {
		return "", "", fmt.Errorf("not inside a task workspace — provide a task ID")
	}

	return taskID, filepath.Join(tasksDir, slug), nil
}
