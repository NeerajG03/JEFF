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
		if bin == "jq" { continue }
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
