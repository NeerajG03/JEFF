package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupStubs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, bin := range []string{"jeff", "gig", "jq"} {
		if bin == "jq" {
			continue
		}
		path := filepath.Join(dir, bin)
		script := "#!/bin/bash\necho 'stub " + bin + "' >&2\n"
		os.WriteFile(path, []byte(script), 0o755)
	}
	if jqPath, err := exec.LookPath("jq"); err == nil {
		jqBytes, _ := os.ReadFile(jqPath)
		os.WriteFile(filepath.Join(dir, "jq"), jqBytes, 0o755)
	}
	return dir
}

func TestScriptExecution(t *testing.T) {
	requireBinaries(t, "bash", "jq")
	stubDir := setupStubs(t)
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", stubDir+":"+oldPath)

	scripts := []struct {
		name string
		gen  func() string
		in   string
	}{
		{"task-context", func() string {
			return taskContextHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
		}, "{}"},
		{"inbox-replay", func() string {
			return inboxReplayHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
		}, "{}"},
		{"worker-heartbeat", func() string {
			return workerHeartbeatHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
		}, "{}"},
		{"session-capture", func() string {
			return sessionCaptureHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
		}, "{\"session_id\": \"mock-sess-123\"}"},
		{"checkpoint-nudge", func() string {
			return checkpointNudgeHook().Scripts["claude"](HookContext{CheckpointPatterns: []string{"git commit"}})
		}, "{\"tool_input\": {\"command\": \"git commit -m 'hi'\"}}"},
		{"gig-ready-tasks", func() string {
			return gigReadyTasksHook().Scripts["claude"](HookContext{})
		}, "{}"},
	}

	for _, s := range scripts {
		t.Run(s.name, func(t *testing.T) {
			content := s.gen()
			scriptPath := filepath.Join(t.TempDir(), s.name+".sh")
			os.WriteFile(scriptPath, []byte(content), 0o755)

			exec.Command("cat", "-v", scriptPath).Run()
			cmd := exec.Command("bash", scriptPath)
			cmd.Stdin = strings.NewReader(s.in)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("script failed: %v\noutput: %s", err, out)
			}
			if !json.Valid(out) {
				t.Errorf("output is not valid JSON: %s", out)
			}
		})
	}
}

func TestAdversarial(t *testing.T) {
	requireBinaries(t, "bash", "jq")
	stubDir := setupStubs(t)
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", stubDir+":"+oldPath)

	adversarialID := "gig-x'; rm -rf $$HOME; echo '"
	adversarialPatterns := []string{"don't", `a"b`}

	scripts := []struct {
		name string
		gen  func() string
	}{
		{"inbox-replay", func() string {
			return inboxReplayHook().Scripts["claude"](HookContext{TaskID: adversarialID})
		}},
		{"checkpoint-nudge", func() string {
			return checkpointNudgeHook().Scripts["claude"](HookContext{CheckpointPatterns: adversarialPatterns})
		}},
	}

	for _, s := range scripts {
		t.Run(s.name, func(t *testing.T) {
			content := s.gen()
			scriptPath := filepath.Join(t.TempDir(), s.name+".sh")
			os.WriteFile(scriptPath, []byte(content), 0o755)

			cmd := exec.Command("bash", "-n", scriptPath)
			if err := cmd.Run(); err != nil {
				t.Errorf("bash -n failed (script has syntax error): %v\n%s", err, content)
			}

			cmd = exec.Command("bash", scriptPath)
			cmd.Stdin = strings.NewReader("{\"tool_input\": {\"command\": \"don't\"}}")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("script failed: %v\noutput: %s", err, out)
			}
			if !json.Valid(out) {
				t.Errorf("output is not valid JSON: %s", out)
			}
		})
	}
}

// runHookScript writes script to a temp file, runs it under bash feeding stdin,
// and returns combined output. Fails the test if the script exits non-zero.
func runHookScript(t *testing.T, script, stdin string) []byte {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\noutput: %s", err, out)
	}
	return out
}

// assertPostToolUseJSON asserts out is valid JSON and is either the empty object
// `{}` or carries hookSpecificOutput.hookEventName == "PostToolUse". This is the
// exact contract Claude Code's PostToolUse output validator enforces — any other
// shape (including empty stdout) fails with "(root): Invalid input".
func assertPostToolUseJSON(t *testing.T, out []byte) {
	t.Helper()
	if len(out) == 0 {
		t.Fatalf("empty stdout is not valid JSON (PostToolUse validation would fail)")
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON: %q", out)
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%q)", err, out)
	}
	hso, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		// No hookSpecificOutput — must be the empty object {}.
		if len(resp) != 0 {
			t.Errorf("expected `{}` or hookEventName==PostToolUse, got: %q", out)
		}
		return
	}
	if ev, _ := hso["hookEventName"].(string); ev != "PostToolUse" {
		t.Errorf("hookEventName = %q, want %q (out=%q)", ev, "PostToolUse", out)
	}
}

// TestCheckpointNudgeNoOpPathsEmitJSON is the regression test for gig-35e2: the
// PostToolUse:Bash checkpoint-nudge hook must emit valid JSON (`{}`), not empty
// stdout, on the no-op paths (command that does NOT match the patterns, and an
// empty command). Empty stdout tripped Claude Code's validator on nearly every
// Bash call.
func TestCheckpointNudgeNoOpPathsEmitJSON(t *testing.T) {
	requireBinaries(t, "bash", "jq")
	stubDir := setupStubs(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	script := checkpointNudgeHook().Scripts["claude"](HookContext{CheckpointPatterns: []string{"git commit"}})

	cases := []struct {
		name string
		in   string
	}{
		{"no-match", `{"tool_input": {"command": "ls -la /tmp"}}`},
		{"empty-command", `{"tool_input": {"command": ""}}`},
		{"missing-command-field", `{"tool_input": {}}`},
		{"match", `{"tool_input": {"command": "git commit -m hi"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := runHookScript(t, script, c.in)
			assertPostToolUseJSON(t, out)
		})
	}
}

// TestPostToolUseGeneratorsAlwaysValidJSON asserts that every PostToolUse script
// generator emits parseable JSON that is `{}` or hookEventName=="PostToolUse" on
// all branches (match / no-match / empty command). No branch may emit empty
// stdout or a mismatched hookEventName.
func TestPostToolUseGeneratorsAlwaysValidJSON(t *testing.T) {
	requireBinaries(t, "bash", "jq")
	stubDir := setupStubs(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	ctx := HookContext{TaskID: "gig-123", CheckpointPatterns: []string{"git commit"}}
	inputs := []struct {
		name string
		in   string
	}{
		{"match-command", `{"tool_input": {"command": "git commit -m hi"}}`},
		{"nomatch-command", `{"tool_input": {"command": "echo hello"}}`},
		{"empty-command", `{"tool_input": {"command": ""}}`},
		{"empty-input", `{}`},
	}

	for _, h := range builtinHooks() {
		if h.Event != "PostToolUse" {
			continue
		}
		gen, ok := h.Scripts["claude"]
		if !ok {
			continue
		}
		script := gen(ctx)
		for _, in := range inputs {
			t.Run(h.Name+"/"+in.name, func(t *testing.T) {
				out := runHookScript(t, script, in.in)
				assertPostToolUseJSON(t, out)
			})
		}
	}
}

// TestPostToolUseGeneratorsValidWithoutJq asserts the jq-missing fallback branch
// of every PostToolUse generator still emits valid JSON (`{}`), guarding the
// secondary concern in gig-35e2 (a jq-missing fallback emitting the wrong shape).
func TestPostToolUseGeneratorsValidWithoutJq(t *testing.T) {
	requireBinaries(t, "bash")
	dir := t.TempDir()
	// Provide jeff/gig stubs and bash, but deliberately NO jq on PATH.
	for _, bin := range []string{"jeff", "gig"} {
		_ = os.WriteFile(filepath.Join(dir, bin), []byte("#!/bin/bash\necho 'stub "+bin+"' >&2\n"), 0o755)
	}
	// Symlink core shell utilities the scripts rely on (but not jq).
	for _, bin := range []string{"bash", "cat"} {
		if p, err := exec.LookPath(bin); err == nil {
			_ = os.Symlink(p, filepath.Join(dir, bin))
		}
	}
	t.Setenv("PATH", dir)

	ctx := HookContext{TaskID: "gig-123", CheckpointPatterns: []string{"git commit"}}
	for _, h := range builtinHooks() {
		if h.Event != "PostToolUse" {
			continue
		}
		gen, ok := h.Scripts["claude"]
		if !ok {
			continue
		}
		script := gen(ctx)
		t.Run(h.Name, func(t *testing.T) {
			out := runHookScript(t, script, `{"tool_input": {"command": "git commit -m hi"}}`)
			assertPostToolUseJSON(t, out)
		})
	}
}

func TestNoJq(t *testing.T) {
	requireBinaries(t, "bash")
	dir := t.TempDir()
	for _, bin := range []string{"jeff", "gig"} {
		path := filepath.Join(dir, bin)
		script := "#!/bin/bash\necho 'stub " + bin + "' >&2\n"
		_ = os.WriteFile(path, []byte(script), 0o755)
	}
	if bashPath, err := exec.LookPath("bash"); err == nil {
		_ = os.Symlink(bashPath, filepath.Join(dir, "bash"))
	}
	t.Setenv("PATH", dir)

	gen := taskContextHook().Scripts["claude"]
	content := gen(HookContext{TaskID: "gig-123"})
	scriptPath := filepath.Join(t.TempDir(), "test.sh")
	os.WriteFile(scriptPath, []byte(content), 0o755)

	exec.Command("cat", "-v", scriptPath).Run()
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = strings.NewReader(`{}`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed without jq: %v\noutput: %s", err, out)
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, out)
	}
	hso, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("expected hookSpecificOutput")
	}
	ctx, _ := hso["additionalContext"].(string)
	if !strings.Contains(ctx, "jq not installed - hooks degraded") {
		t.Errorf("expected degraded context, got: %v", ctx)
	}
}
