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

// CreateContextAliases creates symlinks from each alias filename to CLAUDE.md
// in the given directory. CLAUDE.md is the single source of truth; other
// context files (e.g. GEMINI.md) are symlinks to it.
func CreateContextAliases(dir string, aliases []string) error {
	for _, alias := range aliases {
		link := filepath.Join(dir, alias)
		target := "CLAUDE.md"

		// If symlink already points to the right target, skip.
		if existing, err := os.Readlink(link); err == nil && existing == target {
			continue
		}

		// Remove any existing file/symlink.
		os.Remove(link)

		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s -> %s: %w", alias, target, err)
		}
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
