package jeff

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// isCodexModel returns true if the model name belongs to the OpenAI/Codex family.
// Accepts aliases (terra, luna) and model prefixes (gpt-5*, gpt-4.5*, codex-*, o1*, o3*, o4*).
func isCodexModel(m string) bool {
	switch m {
	case "terra", "luna":
		return true
	}
	if strings.HasPrefix(m, "gpt-5") ||
		strings.HasPrefix(m, "gpt-4.5") ||
		strings.HasPrefix(m, "codex-") ||
		strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") {
		return true
	}
	return false
}

// codexProvider implements AgentProvider for OpenAI Codex CLI.
type codexProvider struct{}

func init() {
	RegisterProvider(&codexProvider{})
}

func (c *codexProvider) Name() AgentTool { return AgentCodex }
func (c *codexProvider) Command() string { return "codex" }

func (c *codexProvider) BuildLaunchArgs(opts LaunchOpts) []string {
	args := []string{}
	if opts.SkipPermissions {
		args = append(args,
			"--sandbox", "workspace-write",
			"--ask-for-approval", "on-request",
			"--dangerously-bypass-hook-trust",
			"--disable", "memories",
		)
	} else {
		args = append(args,
			"--sandbox", "workspace-write",
			"--ask-for-approval", "on-request",
			"--disable", "memories",
		)
	}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	if opts.ResumeSessionID != "" {
		args = append(args, "resume", opts.ResumeSessionID)
	}
	if opts.Prompt != "" && opts.ResumeSessionID == "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// BuildCurateArgs returns CLI args for a non-interactive (piped prompt) session.
func (c *codexProvider) BuildCurateArgs(prompt string, opts LaunchOpts) []string {
	return []string{
		"exec",
		"--sandbox", "workspace-write",
		"--ask-for-approval", "never",
		"--skip-git-repo-check",
		"--disable", "memories",
		prompt,
	}
}

// SupportsInlinePrompt is true: `codex [OPTIONS] [PROMPT]` starts an interactive
// session with the prompt processed as the initial turn.
func (c *codexProvider) SupportsInlinePrompt() bool { return true }
func (c *codexProvider) ConfigDir() string          { return ".codex" }
func (c *codexProvider) SkillsSubdir() string       { return "" }
func (c *codexProvider) CommandsSubdir() string     { return "" }
func (c *codexProvider) CommandFileExt() string     { return "" }

func (c *codexProvider) ContextFileAliases() []string {
	return []string{"AGENTS.md"}
}

func (c *codexProvider) ContextFileName() string { return "AGENTS.md" }

func (c *codexProvider) MemorySuppressEnv() map[string]string {
	return map[string]string{
		"CODEX_DISABLE_MEMORIES": "1",
	}
}

func (c *codexProvider) SendTiming() SendTiming {
	return SendTiming{
		PasteDelay:        500 * time.Millisecond,
		InterruptSettle:   2 * time.Second,
		UseBracketedPaste: true,
		InitialPasteSleep: 3 * time.Second,
	}
}

func (c *codexProvider) OwnsModel(model string) bool { return isCodexModel(model) }

func (c *codexProvider) ModelExamples() []string {
	return []string{"gpt-5.6-terra", "gpt-5.4", "gpt-5", "o3", "o1"}
}

func (c *codexProvider) DoctorDeps() []DoctorDep {
	return []DoctorDep{
		{
			Name:     "codex",
			Required: false,
			Hint:     "npm install -g @openai/codex (or visit https://learn.chatgpt.com/docs/cli)",
		},
	}
}

func (c *codexProvider) EnsureHomeDirs(home string) error {
	return os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
}

func (c *codexProvider) WriteHomeDefaults(home string) error {
	writeIfNotExists(filepath.Join(home, ".codex", "hooks.json"), "{\"hooks\":{}}\n")
	return nil
}

func (c *codexProvider) HookDeliveryKey() string { return "codex" }
