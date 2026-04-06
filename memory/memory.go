// Package memory manages persona memory and repo learnings directories.
package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PersonaMemoryDir returns the absolute path to a persona's memory directory.
func PersonaMemoryDir(jeffHome, personaName string) string {
	return filepath.Join(jeffHome, "personas", personaName, "memory")
}

// RepoLearningsDir returns the absolute path to a repo's learnings directory.
func RepoLearningsDir(jeffHome, repoName string) string {
	return filepath.Join(jeffHome, "learnings", repoName)
}

// ScratchpadPath returns the path to the scratchpad file in a task directory.
func ScratchpadPath(taskDir string) string {
	return filepath.Join(taskDir, "scratchpad.md")
}

// EnsurePersonaDir creates the persona memory directory and a seed MEMORY.md
// if they don't already exist.
func EnsurePersonaDir(jeffHome, personaName string) error {
	dir := PersonaMemoryDir(jeffHome, personaName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create persona memory dir: %w", err)
	}

	indexPath := filepath.Join(dir, "MEMORY.md")
	if _, err := os.Stat(indexPath); err == nil {
		return nil // already exists
	}

	name := strings.ToUpper(personaName[:1]) + personaName[1:]
	seed := fmt.Sprintf("# %s Memory\n\n<!-- Add entries as: - [Title](file.md) — one-line summary -->\n", name)
	return os.WriteFile(indexPath, []byte(seed), 0o644)
}

// EnsureRepoDir creates the repo learnings directory and a seed INDEX.md
// if they don't already exist.
func EnsureRepoDir(jeffHome, repoName string) error {
	dir := RepoLearningsDir(jeffHome, repoName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create repo learnings dir: %w", err)
	}

	indexPath := filepath.Join(dir, "INDEX.md")
	if _, err := os.Stat(indexPath); err == nil {
		return nil // already exists
	}

	seed := fmt.Sprintf("# %s Learnings\n\n<!-- Add entries as: - [Title](file.md) — one-line summary -->\n",
		repoName)
	return os.WriteFile(indexPath, []byte(seed), 0o644)
}

// LoadPersonaMemory reads the MEMORY.md content for a persona.
// Returns empty string if the file doesn't exist or only contains the seed template.
func LoadPersonaMemory(jeffHome, personaName string) (string, error) {
	return loadIndex(filepath.Join(PersonaMemoryDir(jeffHome, personaName), "MEMORY.md"))
}

// LoadRepoLearnings reads the INDEX.md content for a repo.
// Returns empty string if the file doesn't exist or only contains the seed template.
func LoadRepoLearnings(jeffHome, repoName string) (string, error) {
	return loadIndex(filepath.Join(RepoLearningsDir(jeffHome, repoName), "INDEX.md"))
}

// loadIndex reads an index file and returns its content.
// Returns "" if the file is missing or only contains the seed comment.
func loadIndex(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	content := strings.TrimSpace(string(data))

	// If the file only has the seed template (heading + comment), treat as empty.
	lines := strings.Split(content, "\n")
	nonEmpty := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "<!--") || strings.HasSuffix(trimmed, "-->") {
			continue
		}
		nonEmpty++
	}
	if nonEmpty == 0 {
		return "", nil
	}

	return content, nil
}

// InstallLearnCommand writes the /learn slash command to the task directory.
// The command template has all paths baked in so it works when invoked.
// AutoCurate reads the scratchpad and runs the /learn curation prompt
// via a non-interactive agent invocation. Returns nil if no scratchpad
// content exists. The task directory must still exist when this is called.
func AutoCurate(taskDir, taskID, personaName, jeffHome string, repos []string) error {
	sp := ScratchpadPath(taskDir)
	data, err := os.ReadFile(sp)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil // nothing to curate
	}

	// Read the /learn command template.
	learnPath := filepath.Join(taskDir, ".claude", "commands", "learn.md")
	prompt, err := os.ReadFile(learnPath)
	if err != nil {
		return fmt.Errorf("read learn command: %w", err)
	}

	// Launch claude in non-interactive mode with the curation prompt.
	cmd := exec.Command("claude", "--dangerously-skip-permissions", "-p", string(prompt))
	cmd.Dir = taskDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func InstallLearnCommand(taskDir, taskID, personaName, jeffHome string, repos []string) error {
	dir := filepath.Join(taskDir, ".claude", "commands")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create commands dir: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Process the scratchpad from this task and curate observations into persistent memory.\n\n")

	sb.WriteString("## Context\n\n")
	sb.WriteString(fmt.Sprintf("- **Task:** %s\n", taskID))
	sb.WriteString(fmt.Sprintf("- **Scratchpad:** %s\n", ScratchpadPath(taskDir)))
	if personaName != "" {
		sb.WriteString(fmt.Sprintf("- **Persona:** %s\n", personaName))
		sb.WriteString(fmt.Sprintf("- **Persona memory:** %s\n", PersonaMemoryDir(jeffHome, personaName)))
	}
	for _, repo := range repos {
		sb.WriteString(fmt.Sprintf("- **Repo learnings (%s):** %s\n", repo, RepoLearningsDir(jeffHome, repo)))
	}
	sb.WriteString("\n")

	sb.WriteString(`## Steps

1. Read the scratchpad file listed above
2. Read the latest checkpoint via ` + "`gig show " + taskID + "`" + `
3. For each observation in the scratchpad, determine:
   - Is this persona-scoped or repo-scoped? (look for [persona] or [repo:<name>] tags)
   - Is this a new entry or an update to an existing one?
   - Is this actionable and useful for future sessions?
`)

	if personaName != "" {
		sb.WriteString(fmt.Sprintf(`4. For persona-scoped learnings:
   - Read the current MEMORY.md at %s/MEMORY.md
   - Create or update detail files in that directory
   - Update the MEMORY.md index with one-line entries
`, PersonaMemoryDir(jeffHome, personaName)))
	}

	sb.WriteString("5. For repo-scoped learnings:\n")
	for _, repo := range repos {
		sb.WriteString(fmt.Sprintf("   - **%s:** Read INDEX.md at %s/INDEX.md, create/update detail files, update index\n",
			repo, RepoLearningsDir(jeffHome, repo)))
	}

	sb.WriteString(`6. Present a summary of what was added/updated for the user to review

## Memory File Format

Detail files use frontmatter:
` + "```markdown\n---\nsource: " + taskID + "\nupdated: YYYY-MM-DD\n---\n<actionable content>\n```" + `

Index entries (MEMORY.md / INDEX.md) are one line each:
` + "```\n- [Title](file.md) — one-line summary\n```" + `

## Rules
- Entries must be actionable ("do X when Y"), not narrative ("we did X")
- Merge with existing entries rather than duplicating
- Keep MEMORY.md/INDEX.md under 200 lines
- Mistakes and user corrections get priority
- When in doubt, scope to repo learnings (broader audience)
`)

	path := filepath.Join(dir, "learn.md")
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
