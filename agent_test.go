package jeff

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAllProvidersRegistered(t *testing.T) {
	for _, agent := range RegisteredAgents() {
		p := GetProvider(agent)
		if p == nil {
			t.Errorf("no provider registered for %s", agent)
			continue
		}
		if p.Name() != agent {
			t.Errorf("provider Name() = %s, want %s", p.Name(), agent)
		}
	}
}

func TestRegisteredAgents(t *testing.T) {
	agents := RegisteredAgents()
	if len(agents) < 3 {
		t.Fatalf("expected at least 3 registered agents, got %d", len(agents))
	}
	for _, expected := range []AgentTool{AgentClaudeCode, AgentOpenCode, AgentGemini} {
		if !slices.Contains(agents, expected) {
			t.Errorf("RegisteredAgents() missing %s", expected)
		}
	}
}

func TestClaudeProviderArgs(t *testing.T) {
	p := GetProvider(AgentClaudeCode)

	// Basic launch (no options).
	args := p.BuildLaunchArgs(LaunchOpts{})
	if len(args) != 1 || args[0] != "--dangerously-skip-permissions" {
		t.Errorf("basic launch args = %v", args)
	}

	// Launch with model.
	args = p.BuildLaunchArgs(LaunchOpts{Model: "opus"})
	if !slices.Contains(args, "--model") || !slices.Contains(args, "opus") {
		t.Errorf("model args = %v", args)
	}

	// Launch with resume.
	args = p.BuildLaunchArgs(LaunchOpts{ResumeSessionID: "abc-123"})
	if !slices.Contains(args, "--resume") || !slices.Contains(args, "abc-123") {
		t.Errorf("resume args = %v", args)
	}

	// Launch with inline prompt.
	args = p.BuildLaunchArgs(LaunchOpts{Prompt: "do the thing"})
	if args[len(args)-1] != "do the thing" {
		t.Errorf("prompt args = %v", args)
	}

	// Curate args.
	curate := p.BuildCurateArgs("curate this")
	if len(curate) != 3 || curate[1] != "-p" || curate[2] != "curate this" {
		t.Errorf("curate args = %v", curate)
	}
}

func TestGeminiProviderArgs(t *testing.T) {
	p := GetProvider(AgentGemini)

	// Basic launch.
	args := p.BuildLaunchArgs(LaunchOpts{})
	if len(args) != 1 || args[0] != "--approval-mode=yolo" {
		t.Errorf("basic launch args = %v", args)
	}

	// Launch with model.
	args = p.BuildLaunchArgs(LaunchOpts{Model: "gemini-pro"})
	if !slices.Contains(args, "-m") || !slices.Contains(args, "gemini-pro") {
		t.Errorf("model args = %v", args)
	}

	// Resume uses "latest".
	args = p.BuildLaunchArgs(LaunchOpts{ResumeSessionID: "anything"})
	if !slices.Contains(args, "--resume") || !slices.Contains(args, "latest") {
		t.Errorf("resume args = %v", args)
	}

	// Curate args (must include --approval-mode=yolo).
	curate := p.BuildCurateArgs("curate this")
	if len(curate) != 3 || curate[0] != "--approval-mode=yolo" || curate[1] != "-p" || curate[2] != "curate this" {
		t.Errorf("curate args = %v", curate)
	}

	// Context file aliases.
	aliases := p.ContextFileAliases()
	if len(aliases) != 1 || aliases[0] != "GEMINI.md" {
		t.Errorf("context aliases = %v", aliases)
	}

	// Command file ext.
	if p.CommandFileExt() != "toml" {
		t.Errorf("command ext = %s", p.CommandFileExt())
	}
}

func TestOpenCodeProviderArgs(t *testing.T) {
	p := GetProvider(AgentOpenCode)

	// No prompt = nil args.
	args := p.BuildLaunchArgs(LaunchOpts{})
	if args != nil {
		t.Errorf("basic launch args = %v, want nil", args)
	}

	// With prompt.
	args = p.BuildLaunchArgs(LaunchOpts{Prompt: "hello"})
	if len(args) != 2 || args[0] != "--prompt" || args[1] != "hello" {
		t.Errorf("prompt args = %v", args)
	}

	// Curate not supported.
	if p.BuildCurateArgs("x") != nil {
		t.Error("opencode should not support curate")
	}
}

func TestProviderLayout(t *testing.T) {
	tests := []struct {
		agent      AgentTool
		configDir  string
		skillsSub  string
		commandSub string
		cmdExt     string
	}{
		{AgentClaudeCode, ".claude", "skills", "commands", "md"},
		{AgentOpenCode, ".opencode", "", "", ""},
		{AgentGemini, ".gemini", "skills", "commands", "toml"},
	}

	for _, tt := range tests {
		p := GetProvider(tt.agent)
		if p.ConfigDir() != tt.configDir {
			t.Errorf("%s ConfigDir() = %s, want %s", tt.agent, p.ConfigDir(), tt.configDir)
		}
		if p.SkillsSubdir() != tt.skillsSub {
			t.Errorf("%s SkillsSubdir() = %s, want %s", tt.agent, p.SkillsSubdir(), tt.skillsSub)
		}
		if p.CommandsSubdir() != tt.commandSub {
			t.Errorf("%s CommandsSubdir() = %s, want %s", tt.agent, p.CommandsSubdir(), tt.commandSub)
		}
		if p.CommandFileExt() != tt.cmdExt {
			t.Errorf("%s CommandFileExt() = %s, want %s", tt.agent, p.CommandFileExt(), tt.cmdExt)
		}
	}
}

func TestEnsureHomeDirs(t *testing.T) {
	home := t.TempDir()
	for _, agent := range RegisteredAgents() {
		p := GetProvider(agent)
		if err := p.EnsureHomeDirs(home); err != nil {
			t.Errorf("%s EnsureHomeDirs: %v", agent, err)
		}
		dir := filepath.Join(home, p.ConfigDir())
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s config dir not created: %v", agent, err)
		}
	}
}

func TestWriteHomeDefaults(t *testing.T) {
	home := t.TempDir()
	for _, agent := range RegisteredAgents() {
		p := GetProvider(agent)
		_ = p.EnsureHomeDirs(home)
		if err := p.WriteHomeDefaults(home); err != nil {
			t.Errorf("%s WriteHomeDefaults: %v", agent, err)
		}
		settings := filepath.Join(home, p.ConfigDir(), "settings.json")
		if _, err := os.Stat(settings); err != nil {
			t.Errorf("%s settings.json not created: %v", agent, err)
		}
	}
}

func TestAgentToolCommandViaProvider(t *testing.T) {
	if AgentClaudeCode.Command() != "claude" {
		t.Error("claude Command() mismatch")
	}
	if AgentGemini.Command() != "gemini" {
		t.Error("gemini Command() mismatch")
	}
	if AgentOpenCode.Command() != "opencode" {
		t.Error("opencode Command() mismatch")
	}
}

func TestInferBackend(t *testing.T) {
	tests := []struct {
		model string
		want  AgentTool
	}{
		// Claude aliases
		{"sonnet", AgentClaudeCode},
		{"opus", AgentClaudeCode},
		{"haiku", AgentClaudeCode},
		// Claude full IDs
		{"claude-3-5-sonnet-20241022", AgentClaudeCode},
		{"claude-opus-4-7", AgentClaudeCode},
		// Gemini aliases
		{"pro", AgentGemini},
		{"flash", AgentGemini},
		{"flash-lite", AgentGemini},
		{"auto", AgentGemini},
		// Gemini full IDs
		{"gemini-2.0-flash", AgentGemini},
		{"gemini-3-pro-preview", AgentGemini},
		// Unknown
		{"bogus", ""},
		{"gpt-4", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := InferBackend(tt.model)
		if got != tt.want {
			t.Errorf("InferBackend(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestIsValidModel(t *testing.T) {
	tests := []struct {
		agent AgentTool
		model string
		want  bool
	}{
		{AgentClaudeCode, "sonnet", true},
		{AgentClaudeCode, "opus", true},
		{AgentClaudeCode, "haiku", true},
		{AgentClaudeCode, "claude-3-5-sonnet-20241022", true},
		{AgentClaudeCode, "flash", false},
		{AgentClaudeCode, "bogus", false},
		{AgentGemini, "pro", true},
		{AgentGemini, "flash", true},
		{AgentGemini, "flash-lite", true},
		{AgentGemini, "auto", true},
		{AgentGemini, "gemini-2.0-flash", true},
		{AgentGemini, "opus", false},
		{AgentGemini, "bogus", false},
		{AgentOpenCode, "anything", false},
	}
	for _, tt := range tests {
		got := IsValidModel(tt.agent, tt.model)
		if got != tt.want {
			t.Errorf("IsValidModel(%q, %q) = %v, want %v", tt.agent, tt.model, got, tt.want)
		}
	}
}

func TestInferBackendCrossRouting(t *testing.T) {
	// A known claude model on a gemini-default persona should route to claude.
	personaDefault := AgentGemini
	model := "opus"
	if inferred := InferBackend(model); inferred != "" {
		personaDefault = inferred
	}
	if personaDefault != AgentClaudeCode {
		t.Errorf("cross-routing: gemini-default persona + model=opus should yield claude, got %q", personaDefault)
	}

	// A known gemini model on a claude-default persona should route to gemini.
	personaDefault = AgentClaudeCode
	model = "flash"
	if inferred := InferBackend(model); inferred != "" {
		personaDefault = inferred
	}
	if personaDefault != AgentGemini {
		t.Errorf("cross-routing: claude-default persona + model=flash should yield gemini, got %q", personaDefault)
	}
}

func TestUnknownModelError(t *testing.T) {
	msg := UnknownModelError("bogus")
	if !strings.Contains(msg, "bogus") {
		t.Error("error message should contain the unknown model name")
	}
	if !strings.Contains(msg, "sonnet") {
		t.Error("error message should list valid claude models")
	}
	if !strings.Contains(msg, "flash") {
		t.Error("error message should list valid gemini models")
	}
}
