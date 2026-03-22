package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NeerajG03/JEFF/internal/gitutil"
)

// skillsDir returns the .claude/skills path inside a task directory.
func skillsDir(taskDir string) string {
	return filepath.Join(taskDir, ".claude", "skills")
}

// Inject creates a symlink from taskDir/.claude/skills/<name> → skillLocation.
// If the symlink already points to the same target, this is a no-op.
func Inject(skillName, skillLocation, taskDir string) error {
	if _, err := os.Stat(skillLocation); err != nil {
		return fmt.Errorf("skill location not found: %s", skillLocation)
	}

	dir := skillsDir(taskDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .claude/skills: %w", err)
	}

	link := filepath.Join(dir, skillName)

	// If symlink already exists, check target.
	if gitutil.IsSymlink(link) {
		target, err := os.Readlink(link)
		if err == nil && target == skillLocation {
			return nil // already correct
		}
		os.Remove(link) // different target, replace
	} else if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("%s exists and is not a symlink", link)
	}

	return os.Symlink(skillLocation, link)
}

// Eject removes a skill symlink from a task directory. Idempotent.
func Eject(skillName, taskDir string) error {
	link := filepath.Join(skillsDir(taskDir), skillName)
	if !gitutil.IsSymlink(link) {
		return nil
	}
	return os.Remove(link)
}

// Injected returns the names of skills currently symlinked in the task directory.
func Injected(taskDir string) ([]string, error) {
	dir := skillsDir(taskDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		fullPath := filepath.Join(dir, e.Name())
		if gitutil.IsSymlink(fullPath) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// InjectMatching loads the skill registry, finds all skills matching the
// context, and injects them into the task directory via symlinks.
// Returns the names of injected skills.
func InjectMatching(jeffHome, taskDir string, ctx *MatchContext) ([]string, error) {
	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return nil, err
	}

	names := MatchAll(sc, ctx)
	var injected []string
	for _, name := range names {
		entry := sc.Skills[name]
		if err := Inject(name, entry.Location, taskDir); err != nil {
			return injected, fmt.Errorf("inject %s: %w", name, err)
		}
		injected = append(injected, name)
	}
	return injected, nil
}
