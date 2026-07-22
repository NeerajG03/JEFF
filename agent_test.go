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

	// Basic launch (no options): SkipPermissions defaults false at this layer.
	args := p.BuildLaunchArgs(LaunchOpts{})
	if len(args) != 0 {
		t.Errorf("basic launch args = %v, want []", args)
	}

	// SkipPermissions true adds the flag.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true})
	if len(args) != 1 || args[0] != "--dangerously-skip-permissions" {
		t.Errorf("skip-permissions args = %v", args)
	}

	// SkipPermissions false must never include the flag.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: false, Model: "opus"})
	if slices.Contains(args, "--dangerously-skip-permissions") {
		t.Errorf("args with SkipPermissions:false must not contain --dangerously-skip-permissions, got %v", args)
	}

	// Launch with model.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, Model: "opus"})
	if !slices.Contains(args, "--model") || !slices.Contains(args, "opus") {
		t.Errorf("model args = %v", args)
	}

	// Launch with resume.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, ResumeSessionID: "abc-123"})
	if !slices.Contains(args, "--resume") || !slices.Contains(args, "abc-123") {
		t.Errorf("resume args = %v", args)
	}

	// Launch with inline prompt.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, Prompt: "do the thing"})
	if args[len(args)-1] != "do the thing" {
		t.Errorf("prompt args = %v", args)
	}

	// Curate args always include the skip flag regardless of opts.
	curate := p.BuildCurateArgs("curate this", LaunchOpts{SkipPermissions: false})
	if len(curate) != 3 || curate[0] != "--dangerously-skip-permissions" || curate[1] != "-p" || curate[2] != "curate this" {
		t.Errorf("curate args = %v", curate)
	}
}

func TestGeminiProviderArgs(t *testing.T) {
	p := GetProvider(AgentGemini)

	// Basic launch: SkipPermissions defaults false at this layer.
	args := p.BuildLaunchArgs(LaunchOpts{})
	if len(args) != 0 {
		t.Errorf("basic launch args = %v, want []", args)
	}

	// SkipPermissions true adds the flag.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true})
	if len(args) != 1 || args[0] != "--approval-mode=yolo" {
		t.Errorf("skip-permissions args = %v", args)
	}

	// SkipPermissions false must never include the flag.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: false, Model: "gemini-pro"})
	if slices.Contains(args, "--approval-mode=yolo") {
		t.Errorf("args with SkipPermissions:false must not contain --approval-mode=yolo, got %v", args)
	}

	// Launch with model.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, Model: "gemini-pro"})
	if !slices.Contains(args, "-m") || !slices.Contains(args, "gemini-pro") {
		t.Errorf("model args = %v", args)
	}

	// "flash" alias pins to gemini-3.5-flash.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, Model: "flash"})
	if !slices.Contains(args, "-m") || !slices.Contains(args, "gemini-3.5-flash") {
		t.Errorf("flash alias should resolve to gemini-3.5-flash, args = %v", args)
	}
	if slices.Contains(args, "flash") {
		t.Errorf("flash alias should not be passed as bare 'flash', args = %v", args)
	}

	// Other aliases pass through unchanged.
	for _, alias := range []string{"pro", "flash-lite", "auto"} {
		args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, Model: alias})
		if !slices.Contains(args, "-m") || !slices.Contains(args, alias) {
			t.Errorf("alias %q should pass through, args = %v", alias, args)
		}
	}

	// Resume uses "latest".
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, ResumeSessionID: "anything"})
	if !slices.Contains(args, "--resume") || !slices.Contains(args, "latest") {
		t.Errorf("resume args = %v", args)
	}

	// Curate args always include --approval-mode=yolo regardless of opts.
	curate := p.BuildCurateArgs("curate this", LaunchOpts{SkipPermissions: false})
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

	// Default args: SkipPermissions false at this layer means no --auto.
	args := p.BuildLaunchArgs(LaunchOpts{})
	if len(args) != 0 {
		t.Errorf("basic launch args = %v, want []", args)
	}

	// SkipPermissions true adds --auto.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true})
	if len(args) != 1 || args[0] != "--auto" {
		t.Errorf("skip-permissions args = %v, want [--auto]", args)
	}

	// With prompt.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, Prompt: "hello"})
	if len(args) != 3 || args[0] != "--auto" || args[1] != "--prompt" || args[2] != "hello" {
		t.Errorf("prompt args = %v, want [--auto --prompt hello]", args)
	}

	// AgentName must NOT become --agent: JEFF personas are not OpenCode agents,
	// and OpenCode would resolve --agent against .opencode/agents/ (unpopulated
	// in task dirs), selecting a nonexistent agent.
	args = p.BuildLaunchArgs(LaunchOpts{SkipPermissions: true, AgentName: "jenko"})
	if slices.Contains(args, "--agent") {
		t.Errorf("opencode args must not contain --agent, got %v", args)
	}

	// Curate is supported via `run --auto`, always, regardless of opts.
	curate := p.BuildCurateArgs("test prompt", LaunchOpts{SkipPermissions: false})
	if len(curate) != 3 || curate[0] != "run" || curate[1] != "--auto" || curate[2] != "test prompt" {
		t.Errorf("curate args = %v, want [run --auto test prompt]", curate)
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
		{AgentOpenCode, ".opencode", "skills", "commands", "md"},
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
		cfgFile := "settings.json"
		if agent == AgentOpenCode {
			cfgFile = "opencode.json"
		}
		cfgPath := filepath.Join(home, p.ConfigDir(), cfgFile)
		if _, err := os.Stat(cfgPath); err != nil {
			t.Errorf("%s %s not created: %v", agent, cfgFile, err)
		}
	}
}

func TestWriteOpenCodeDefaults_RemovesStaleSettings(t *testing.T) {
	home := t.TempDir()
	p := GetProvider(AgentOpenCode)
	_ = p.EnsureHomeDirs(home)

	// Plant stale settings files (old format).
	for _, name := range []string{"settings.json", "settings.local.json"} {
		path := filepath.Join(home, ".opencode", name)
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := p.WriteHomeDefaults(home); err != nil {
		t.Fatalf("WriteHomeDefaults: %v", err)
	}

	// Stale files should be removed.
	for _, name := range []string{"settings.json", "settings.local.json"} {
		path := filepath.Join(home, ".opencode", name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be removed, err=%v", name, err)
		}
	}

	// New config should exist.
	if _, err := os.Stat(filepath.Join(home, ".opencode", "opencode.json")); err != nil {
		t.Error("opencode.json should exist")
	}

	// Second call should be idempotent (no error when files already gone).
	if err := p.WriteHomeDefaults(home); err != nil {
		t.Errorf("second call: %v", err)
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
