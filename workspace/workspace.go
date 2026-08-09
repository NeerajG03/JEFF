// Package workspace manages task directories and their lifecycle.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DefaultTaskIDPrefix is gig's default task-ID prefix. gig treats the prefix as
// configuration (`gig config set prefix`), so parsing code must never assume
// this value — it is only the fallback when no prefix is supplied (#97).
const DefaultTaskIDPrefix = "gig"

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

	// MkdirAll reuses an existing directory, so a task closed and then picked up
	// again lands back in its RETIRED workspace. Clear the marker: leaving it
	// would hide a live workspace from `jeff status` and hook sync, and would let
	// `jeff cleanup` delete it out from under the session once the grace period
	// elapsed. Creating a workspace means it is live.
	Unretire(dir)

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

// List returns all task workspaces. prefix is the gig store's configured
// task-ID prefix ("" means DefaultTaskIDPrefix); it is needed to recover each
// dir's TaskID from its slug.
func List(jeffHome, prefix string) ([]*TaskDir, error) {
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
		taskID := ExtractTaskID(e.Name(), prefix)
		dirs = append(dirs, &TaskDir{
			Path:   filepath.Join(tasksDir, e.Name()),
			TaskID: taskID,
			Slug:   e.Name(),
		})
	}
	return dirs, nil
}

// ListActive returns task workspaces that have not been retired by `jeff done`.
//
// Since `done` keeps a task dir instead of deleting it (#94), "a directory exists"
// no longer means "the task is live". Anything presenting workspaces as current
// work — status, completions, hook sync — must filter on this rather than on List,
// or closed tasks keep showing up as active.
func ListActive(jeffHome, prefix string) ([]*TaskDir, error) {
	all, err := List(jeffHome, prefix)
	if err != nil {
		return nil, err
	}
	var out []*TaskDir
	for _, td := range all {
		if IsRetired(td.Path) {
			continue
		}
		out = append(out, td)
	}
	return out, nil
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

// taskIDPatternCache memoizes the compiled pattern per prefix. ExtractTaskID
// runs once per directory entry in List/ListActive/taskWorkspaces, and the
// prefix is effectively constant within a process (one gig store), so
// recompiling the same pattern on every call is pure waste (#105).
var (
	taskIDPatternCacheMu sync.RWMutex
	taskIDPatternCache   = make(map[string]*regexp.Regexp)
)

func taskIDPatternFor(prefix string) *regexp.Regexp {
	taskIDPatternCacheMu.RLock()
	re, ok := taskIDPatternCache[prefix]
	taskIDPatternCacheMu.RUnlock()
	if ok {
		return re
	}

	re = regexp.MustCompile(`^(` + regexp.QuoteMeta(prefix) + `-[a-z0-9]+(?:\.[0-9]+)*)`)

	taskIDPatternCacheMu.Lock()
	taskIDPatternCache[prefix] = re
	taskIDPatternCacheMu.Unlock()
	return re
}

// ExtractTaskID pulls the gig task ID from a slug or full path
// (e.g., "gig-ab12-some-title" → "gig-ab12", "/path/to/tasks/gig-ab12-foo" → "gig-ab12").
// prefix is the gig store's configured task-ID prefix; "" means
// DefaultTaskIDPrefix. It must be threaded from the store's config, not
// hardcoded: a custom prefix ("cbx-ab12-some-title") otherwise falls through to
// the whole slug and every store lookup fails (#97).
// Returns the slug unchanged when no ID matches.
func ExtractTaskID(slugOrPath, prefix string) string {
	if prefix == "" {
		prefix = DefaultTaskIDPrefix
	}
	// Use just the base name if a full path is given.
	slug := filepath.Base(slugOrPath)
	// Match <prefix>-XXXX or <prefix>-XXXX.N patterns at the start.
	m := taskIDPatternFor(prefix).FindString(slug)
	if m != "" {
		return m
	}
	return slug
}
