// Package workspace manages task directories and their lifecycle.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TaskDir represents an active task workspace.
type TaskDir struct {
	Path   string // absolute path to task directory
	TaskID string // gig task ID
	Slug   string // human-readable slug (e.g., gig-ab12-refactor-auth)
}

// Create creates a new task workspace directory under jeffHome/tasks/.
// Returns the TaskDir with the absolute path.
func Create(jeffHome, taskID, title string) (*TaskDir, error) {
	slug := makeSlug(taskID, title)
	dir := filepath.Join(jeffHome, "tasks", slug)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create task dir: %w", err)
	}

	return &TaskDir{
		Path:   dir,
		TaskID: taskID,
		Slug:   slug,
	}, nil
}

// Open finds an existing task workspace by task ID.
// Searches jeffHome/tasks/ for a directory starting with the task ID.
func Open(jeffHome, taskID string) (*TaskDir, error) {
	tasksDir := filepath.Join(jeffHome, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no task workspace for %s", taskID)
		}
		return nil, fmt.Errorf("read tasks dir: %w", err)
	}

	prefix := taskID + "-"
	for _, e := range entries {
		if e.IsDir() && (e.Name() == taskID || strings.HasPrefix(e.Name(), prefix)) {
			return &TaskDir{
				Path:   filepath.Join(tasksDir, e.Name()),
				TaskID: taskID,
				Slug:   e.Name(),
			}, nil
		}
	}

	return nil, fmt.Errorf("no task workspace for %s", taskID)
}

// Remove deletes a task workspace directory.
func Remove(jeffHome, taskID string) error {
	td, err := Open(jeffHome, taskID)
	if err != nil {
		return err
	}
	return os.RemoveAll(td.Path)
}

// List returns all task workspaces.
func List(jeffHome string) ([]*TaskDir, error) {
	tasksDir := filepath.Join(jeffHome, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tasks dir: %w", err)
	}

	var dirs []*TaskDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Extract task ID: everything before the first dash after the prefix pattern.
		taskID := extractTaskID(e.Name())
		dirs = append(dirs, &TaskDir{
			Path:   filepath.Join(tasksDir, e.Name()),
			TaskID: taskID,
			Slug:   e.Name(),
		})
	}
	return dirs, nil
}

// makeSlug creates a filesystem-safe directory name from task ID and title.
func makeSlug(taskID, title string) string {
	if title == "" {
		return taskID
	}
	// Lowercase, replace non-alphanumeric with dashes, collapse, trim.
	slug := strings.ToLower(title)
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	// Truncate to keep paths reasonable.
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}

	return taskID + "-" + slug
}

// extractTaskID pulls the gig task ID from a slug (e.g., "gig-ab12-some-title" → "gig-ab12").
func extractTaskID(slug string) string {
	// Match gig-XXXX or gig-XXXX.N patterns at the start.
	re := regexp.MustCompile(`^(gig-[a-z0-9]+(?:\.[0-9]+)*)`)
	m := re.FindString(slug)
	if m != "" {
		return m
	}
	return slug
}
