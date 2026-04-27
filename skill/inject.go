package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NeerajG03/JEFF/internal/gitutil"
)

// skillsDir returns the skills path inside a task directory.
// configDir is the agent config dir (e.g. ".claude"), skillsSubdir is "skills".
func skillsDir(taskDir string) string {
	return skillsDirFor(taskDir, ".claude", "skills")
}

// skillsDirFor returns the skills path for a given config dir and subdirectory.
func skillsDirFor(taskDir, configDir, skillsSubdir string) string {
	return filepath.Join(taskDir, configDir, skillsSubdir)
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

// InjectTo creates a symlink from taskDir/<configDir>/<skillsSubdir>/<name> → skillLocation.
// This is the provider-aware variant of Inject.
func InjectTo(skillName, skillLocation, taskDir, configDir, skillsSubdir string) error {
	if skillsSubdir == "" {
		return nil // agent doesn't support skills
	}
	if _, err := os.Stat(skillLocation); err != nil {
		return fmt.Errorf("skill location not found: %s", skillLocation)
	}

	dir := skillsDirFor(taskDir, configDir, skillsSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s/%s: %w", configDir, skillsSubdir, err)
	}

	link := filepath.Join(dir, skillName)

	if gitutil.IsSymlink(link) {
		target, err := os.Readlink(link)
		if err == nil && target == skillLocation {
			return nil
		}
		os.Remove(link)
	} else if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("%s exists and is not a symlink", link)
	}

	return os.Symlink(skillLocation, link)
}

// InjectMatching loads the skill registry, finds all skills matching the
// context, and injects them into the task directory via symlinks.
// Returns the names of injected skills.
func InjectMatching(jeffHome, taskDir string, ctx *MatchContext) ([]string, error) {
	return InjectMatchingTo(jeffHome, taskDir, ".claude", "skills", ctx)
}

// InjectMatchingTo is like InjectMatching but uses the given configDir and skillsSubdir.
func InjectMatchingTo(jeffHome, taskDir, configDir, skillsSubdir string, ctx *MatchContext) ([]string, error) {
	if skillsSubdir == "" {
		return nil, nil // agent doesn't support skills
	}

	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return nil, err
	}

	names := MatchAll(sc, ctx)
	var injected []string
	for _, name := range names {
		entry := sc.Skills[name]
		if err := InjectTo(name, entry.Location, taskDir, configDir, skillsSubdir); err != nil {
			return injected, fmt.Errorf("inject %s: %w", name, err)
		}
		injected = append(injected, name)
	}
	return injected, nil
}
