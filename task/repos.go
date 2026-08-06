package task

import (
	"encoding/json"
	"fmt"

	jeff "github.com/NeerajG03/JEFF"
)

// AddTaskRepo appends repoName to the task's repos attribute, the list
// teardown worktree cleanup and task stats read. Pickup writes the initial
// list; this is the incremental path for repos attached mid-task via
// `jeff worktree add --task-dir`, which previously never updated the attribute,
// so `jeff done` silently left those worktrees (and branches) behind (#98).
//
// Returns true when the attribute was extended, false when the repo was
// already registered.
func AddTaskRepo(store Store, taskID, repoName string) (bool, error) {
	var repos []string
	if attr, err := store.GetAttr(taskID, jeff.AttrRepos); err == nil && attr != nil {
		if err := json.Unmarshal([]byte(attr.Value), &repos); err != nil {
			return false, fmt.Errorf("repos attr on %s is not a JSON array: %w", taskID, err)
		}
	}
	for _, r := range repos {
		if r == repoName {
			return false, nil
		}
	}
	repos = append(repos, repoName)
	reposJSON, err := json.Marshal(repos)
	if err != nil {
		return false, err
	}
	if err := store.SetAttr(taskID, jeff.AttrRepos, string(reposJSON)); err != nil {
		return false, err
	}
	return true, nil
}
