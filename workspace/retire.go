// retire.go — retiring a task workspace instead of deleting it.
//
// A task dir is not merely where a task's files live; it is the running agent
// session's life support. It holds the session's cwd, the hook SCRIPTS
// themselves, and the .claude/settings.json that names those scripts by absolute
// path. Deleting it under a live session therefore breaks every subsequent hook
// and Bash spawn — the session's cwd is gone (which Node reports as ENOENT while
// naming the executable, so it masquerades as a missing /bin/sh) and the scripts
// it was configured to run are gone too.
//
// The costs are wildly asymmetric:
//
//	worktrees/<repo>/<branch>/   ~200 MB–1 GB   a full checkout   → worth reclaiming eagerly
//	tasks/<slug>/                ~20 KB         hooks, settings   → worth keeping
//
// So `jeff done` reclaims the worktree and RETIRES the task dir: it drops the
// symlinks that now dangle and leaves a marker recording when and why. The
// directory is collected later by `jeff cleanup`, once no session needs it.
//
// This removes the whole failure class by construction rather than by detection:
// no cwd sniffing is needed, and it holds however the task was closed — including
// from a second terminal, which cwd inspection could never see.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NeerajG03/JEFF/internal/gitutil"
)

// ClosedMarkerName is the file written into a retired task dir. Its presence is
// the cheap, offline test for "this workspace is done with" — enumerators use it
// to skip retired dirs without querying gig.
const ClosedMarkerName = ".closed"

// ClosedMarker records why and when a workspace was retired.
type ClosedMarker struct {
	TaskID   string    `json:"task_id"`
	Reason   string    `json:"reason"`
	ClosedAt time.Time `json:"closed_at"`
}

// RetireResult reports what retiring changed, for the caller's output.
type RetireResult struct {
	// DanglingRemoved are repo symlinks whose worktree target no longer exists
	// and which were therefore removed.
	DanglingRemoved []string
	// MarkerPath is the .closed file written.
	MarkerPath string
}

// Retire marks a task workspace closed without deleting it: dangling repo
// symlinks are removed (their worktrees are gone, so following them only
// confuses tooling and humans) and a .closed marker is written.
//
// It is deliberately non-destructive beyond those symlinks. The hook scripts,
// .claude/settings.json and CLAUDE.md are left in place so a session anchored
// here keeps working until it exits.
func Retire(taskDir, taskID, reason string) (*RetireResult, error) {
	if _, err := os.Stat(taskDir); err != nil {
		return nil, fmt.Errorf("task dir: %w", err)
	}

	res := &RetireResult{}

	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, fmt.Errorf("read task dir: %w", err)
	}
	for _, e := range entries {
		p := filepath.Join(taskDir, e.Name())
		if !gitutil.IsSymlink(p) {
			continue
		}
		// os.Stat follows the link: an error means the target is gone.
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.Remove(p); err == nil {
			res.DanglingRemoved = append(res.DanglingRemoved, e.Name())
		}
	}

	marker := ClosedMarker{TaskID: taskID, Reason: reason, ClosedAt: time.Now().UTC()}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal closed marker: %w", err)
	}
	data = append(data, '\n')

	res.MarkerPath = filepath.Join(taskDir, ClosedMarkerName)
	if err := os.WriteFile(res.MarkerPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write closed marker: %w", err)
	}
	return res, nil
}

// ReadClosedMarker returns the retirement marker for a task dir, or nil when the
// dir is not retired. A marker that exists but cannot be parsed is reported as
// retired with a zero ClosedAt, so a corrupt marker never makes a dir look live
// (which would leak it forever) — it just falls under the age gate.
func ReadClosedMarker(taskDir string) *ClosedMarker {
	data, err := os.ReadFile(filepath.Join(taskDir, ClosedMarkerName))
	if err != nil {
		return nil
	}
	var m ClosedMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return &ClosedMarker{}
	}
	return &m
}

// IsRetired reports whether a task dir has been retired by `jeff done`.
func IsRetired(taskDir string) bool {
	return ReadClosedMarker(taskDir) != nil
}

// Unretire clears a retirement marker, making a workspace live again. Called when
// a workspace is (re)created, since a closed task can be reopened and picked up
// into the very directory it was retired in.
func Unretire(taskDir string) {
	_ = os.Remove(filepath.Join(taskDir, ClosedMarkerName))
}

// PathContains reports whether probe lies inside dir. Both sides are resolved
// through symlinks before comparison, because a task workspace is routinely
// reached via a symlink and macOS adds /var → /private/var on top; comparing raw
// strings would miss the very cases that matter.
func PathContains(dir, probe string) bool {
	if dir == "" || probe == "" {
		return false
	}
	resolve := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	d, pr := resolve(dir), resolve(probe)
	if d == pr {
		return true
	}
	rel, err := filepath.Rel(d, pr)
	if err != nil {
		return false
	}
	return rel != ".." && !hasDotDotPrefix(rel)
}

// CwdInside reports whether the calling process's working directory is inside
// dir. It checks both the resolved cwd and the logical $PWD, since a shell that
// walked in through a symlink reports the logical path.
func CwdInside(dir string) bool {
	if wd, err := os.Getwd(); err == nil && PathContains(dir, wd) {
		return true
	}
	if pwd := os.Getenv("PWD"); pwd != "" && PathContains(dir, pwd) {
		return true
	}
	return false
}

func hasDotDotPrefix(rel string) bool {
	return len(rel) >= 3 && rel[0] == '.' && rel[1] == '.' && rel[2] == filepath.Separator
}

// DirSize returns the total size of the files under path, following no symlinks.
// Used to report what `jeff cleanup` actually reclaimed — the whole point of
// removing worktrees is space, so the number should be visible.
func DirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
