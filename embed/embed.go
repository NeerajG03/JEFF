// Package embed provides default files shipped with the binary.
package embed

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed CLAUDE.md
var DefaultClaudeMD string

//go:embed claude-settings.json
var DefaultClaudeSettings string

//go:embed personas/marlowe/GOAL.md
var MarloweGoalMD string

//go:embed memory-context-claude.md
var MemoryContextClaude string

//go:embed memory-context-gemini.md
var MemoryContextGemini string

// EnsureGeminiSkillsAlias creates dir/.gemini/skills as a symlink to the
// sibling dir/.claude/skills directory. The link target is relative
// ("../.claude/skills") so it stays valid when the workspace is moved.
//
// .claude/skills is the single source of truth for skill symlinks; .gemini/skills
// aliases it so gemini sessions see the same skills as claude sessions.
//
// Unlike CreateContextAliases (which is gated on the gemini agent being
// registered), this alias is created unconditionally: skills should be in
// sync across agents regardless of which agent the user has configured.
//
// Idempotent: returns cleanly if the link already points to the correct
// target. Replaces a stale symlink. Errors if .gemini/skills exists as a
// non-symlink (file or directory) — the caller must remove it manually.
func EnsureGeminiSkillsAlias(dir string) error {
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		return fmt.Errorf("create .claude/skills: %w", err)
	}
	geminiDir := filepath.Join(dir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o755); err != nil {
		return fmt.Errorf("create .gemini: %w", err)
	}

	link := filepath.Join(geminiDir, "skills")
	target := filepath.Join("..", ".claude", "skills")

	if existing, err := os.Readlink(link); err == nil {
		if existing == target {
			return nil
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove stale .gemini/skills symlink: %w", err)
		}
	} else if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("%s exists and is not a symlink; remove it manually", link)
	}

	return os.Symlink(target, link)
}

// EnsureOpenCodeSkillsAlias creates dir/.opencode/skills as a symlink to the
// sibling dir/.claude/skills directory. The link target is relative
// ("../.claude/skills") so it stays valid when the workspace is moved.
//
// .claude/skills is the single source of truth for skill symlinks; .opencode/skills
// aliases it so opencode sessions see the same skills as claude sessions.
//
// If .opencode/skills exists as an empty directory (from an earlier JEFF
// version that created it with EnsureHomeDirs), it is silently replaced with a
// symlink. Non-empty directories are refused with an error.
func EnsureOpenCodeSkillsAlias(dir string) error {
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		return fmt.Errorf("create .claude/skills: %w", err)
	}
	opencodeDir := filepath.Join(dir, ".opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		return fmt.Errorf("create .opencode: %w", err)
	}

	link := filepath.Join(opencodeDir, "skills")
	target := filepath.Join("..", ".claude", "skills")

	if existing, err := os.Readlink(link); err == nil {
		if existing == target {
			return nil
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove stale .opencode/skills symlink: %w", err)
		}
	} else if fi, err := os.Lstat(link); err == nil {
		if fi.IsDir() {
			if empty, _ := isEmptyDir(link); empty {
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove empty .opencode/skills dir: %w", err)
				}
			} else {
				return fmt.Errorf("%s is a non-empty directory; remove it manually", link)
			}
		} else {
			return fmt.Errorf("%s exists and is not a symlink; remove it manually", link)
		}
	}

	return os.Symlink(target, link)
}

// EnsureCodexSkillsAlias creates dir/.agents/skills as a symlink to the
// sibling dir/.claude/skills directory. The link target is relative
// ("../.claude/skills") so it stays valid when the workspace is moved.
//
// .claude/skills is the single source of truth for skill symlinks; .agents/skills
// aliases it so codex sessions see the same skills as claude sessions.
//
// If .agents/skills exists as an empty directory, it is silently replaced with a
// symlink. Non-empty directories are refused with an error.
func EnsureCodexSkillsAlias(dir string) error {
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		return fmt.Errorf("create .claude/skills: %w", err)
	}
	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("create .agents: %w", err)
	}

	link := filepath.Join(agentsDir, "skills")
	target := filepath.Join("..", ".claude", "skills")

	if existing, err := os.Readlink(link); err == nil {
		if existing == target {
			return nil
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove stale .agents/skills symlink: %w", err)
		}
	} else if fi, err := os.Lstat(link); err == nil {
		if fi.IsDir() {
			if empty, _ := isEmptyDir(link); empty {
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove empty .agents/skills dir: %w", err)
				}
			} else {
				return fmt.Errorf("%s is a non-empty directory; remove it manually", link)
			}
		} else {
			return fmt.Errorf("%s exists and is not a symlink; remove it manually", link)
		}
	}

	return os.Symlink(target, link)
}

// isEmptyDir returns true if dir exists, is a directory, and contains no entries.
func isEmptyDir(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err != nil, nil // err != nil when EOF (no entries) → true
}

// CreateContextAliases creates symlinks from each alias filename to CLAUDE.md
// in the given directory. CLAUDE.md is the single source of truth; other
// context files (e.g. GEMINI.md, AGENTS.md) are symlinks to it.
//
// If an alias path exists as a non-symlink regular file (e.g. a pre-existing,
// user-authored AGENTS.md), it is preserved by renaming it to <alias>.bak
// before creating the symlink. If the .bak file already exists, it refuses to
// overwrite and returns an error. Directories are always refused with an error.
func CreateContextAliases(dir string, aliases []string) error {
	for _, alias := range aliases {
		link := filepath.Join(dir, alias)
		target := "CLAUDE.md"

		fi, err := os.Lstat(link)
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.Symlink(target, link); err != nil {
					return fmt.Errorf("symlink %s -> %s: %w", alias, target, err)
				}
				continue
			}
			return fmt.Errorf("stat %s: %w", link, err)
		}

		// If it's a symlink, verify or refresh target.
		if fi.Mode()&os.ModeSymlink != 0 {
			existing, err := os.Readlink(link)
			if err == nil && existing == target {
				continue
			}
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("remove stale symlink %s: %w", link, err)
			}
			if err := os.Symlink(target, link); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", alias, target, err)
			}
			continue
		}

		// Non-symlink: preserve regular files by backing up; refuse directories.
		if fi.Mode().IsRegular() {
			bak := link + ".bak"
			if _, err := os.Lstat(bak); err == nil {
				return fmt.Errorf("%s exists and is not a symlink, and %s already exists; remove or rename manually", link, bak)
			}
			if err := os.Rename(link, bak); err != nil {
				return fmt.Errorf("backup %s -> %s: %w", link, bak, err)
			}
			if err := os.Symlink(target, link); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", alias, target, err)
			}
			continue
		}

		return fmt.Errorf("%s exists and is not a symlink; remove it manually", link)
	}
	return nil
}

// WriteClaudeMD writes the default CLAUDE.md to the target directory.
// homePath is substituted for {{.Home}} in the template (e.g. "~/.jeff/" or "./jeff/").
// If the file already exists, it is left untouched (user may have edited it).
// Use force=true to overwrite.
func WriteClaudeMD(dir, homePath string, force bool) error {
	path := filepath.Join(dir, "CLAUDE.md")

	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil // already exists, don't overwrite
		}
	}

	content := strings.ReplaceAll(DefaultClaudeMD, "{{.Home}}", homePath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
