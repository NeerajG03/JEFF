package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/JEFF/workspace"
)

// taskWorkspace is a task directory under <home>/tasks whose name carries a
// gig- task ID and is therefore eligible for hook re-sync.
type taskWorkspace struct {
	Name   string // directory basename
	Dir    string // absolute path
	TaskID string // extracted gig- id
}

// taskWorkspaces walks <home>/tasks and returns every immediate subdirectory
// whose name resolves to a gig- task ID. Files and non-gig dirs are skipped; a
// missing tasks/ dir yields nil.
func taskWorkspaces(home string) []taskWorkspace {
	tasksDir := filepath.Join(home, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil
	}
	var out []taskWorkspace
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		taskID := workspace.ExtractTaskID(entry.Name())
		if !strings.HasPrefix(taskID, "gig-") {
			continue
		}
		out = append(out, taskWorkspace{
			Name:   entry.Name(),
			Dir:    filepath.Join(tasksDir, entry.Name()),
			TaskID: taskID,
		})
	}
	return out
}

func syncTaskHooks(cfg *jeff.Config, targetDir, taskID, persona string, repos []string, orchestratorID string) {
	reg := hooks.DefaultRegistry()
	mgr := hooks.NewManager(reg)
	hctx := hooks.HookContext{
		JeffHome:           cfg.Home,
		TargetDir:          targetDir,
		GigHome:            cfg.GigHome,
		TaskID:             taskID,
		OrchestratorID:     orchestratorID,
		CheckpointPatterns: cfg.CheckpointPatterns,
		Persona:            persona,
		Repos:              repos,
	}
	taskEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceTask, reg)
	if len(taskEnabled) > 0 {
		for _, agent := range jeff.RegisteredAgents() {
			p := jeff.GetProvider(agent)
			if p == nil {
				continue
			}
			if err := mgr.Sync(targetDir, taskEnabled, p.HookDeliveryKey(), hctx); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write hooks for %s: %v\n", agent, err)
			}
		}
	}
}
