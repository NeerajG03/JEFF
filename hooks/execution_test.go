package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			dir := t.TempDir()
			scriptPath := filepath.Join(dir, s.name+".sh")
			os.WriteFile(scriptPath, []byte(content), 0o755)

			exec.Command("cat", "-v", scriptPath).Run()
			cmd := exec.Command("bash", scriptPath)
			cmd.Dir = dir // worker-heartbeat writes a $(pwd)-relative sentinel
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
// cmd.Dir is pinned to that same temp dir: a real hook always runs with CWD ==
// the task/home dir, and some scripts (memory-propose-nudge's `$(pwd)/.nudged`
// sentinel) write relative to it — leaving Dir unset would let them write into
// this test binary's own working directory instead.
func runHookScript(t *testing.T, script, stdin string) []byte {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = dir
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

// assertValidHookOutput is the general Rule-1 contract (EPIC.md rule 1): stdout
// must always be parseable JSON, and when hookSpecificOutput.hookEventName is
// present it must equal the hook's declared event. Other shapes with no
// hookSpecificOutput (e.g. `{}`, or memory-propose-nudge's Stop `{"decision":
// "block", ...}`) are valid as long as they parse — this is deliberately looser
// than assertPostToolUseJSON, which also enforces "or empty {}" because every
// PostToolUse hook it checks happens to use that shape.
func assertValidHookOutput(t *testing.T, out []byte, event string) {
	t.Helper()
	if len(out) == 0 {
		t.Fatalf("empty stdout is not valid JSON (%s validation would fail)", event)
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
		return
	}
	if ev, _ := hso["hookEventName"].(string); ev != event {
		t.Errorf("hookEventName = %q, want %q (out=%q)", ev, event, out)
	}
}

// hookContractInputs returns representative stdin payloads for a hook's declared
// event: one that matches whatever the hook acts on, one that matches nothing
// (the no-op path), and garbage. Not every hook distinguishes match/no-op (a
// SessionStart hook fires unconditionally once installed) — feeding it anyway
// is harmless and still exercises the "matches nothing useful" branch some
// hooks have regardless of event (e.g. PENDING=0, empty session_id).
func hookContractInputs(event string) []struct{ name, in string } {
	switch event {
	case "PostToolUse":
		return []struct{ name, in string }{
			{"match", `{"tool_input": {"command": "git commit -m hi"}}`},
			{"no-match", `{"tool_input": {"command": "echo hi"}}`},
			{"empty-input", `{}`},
			{"garbage", `not json`},
		}
	case "SessionStart":
		return []struct{ name, in string }{
			{"with-session-id", `{"session_id": "sess-abc123"}`},
			{"empty-input", `{}`},
			{"garbage", `not json`},
		}
	default: // Stop, and anything else the registry adds later
		return []struct{ name, in string }{
			{"empty-input", `{}`},
			{"garbage", `not json`},
		}
	}
}

// isPipeableBashScript reports whether content is a bash script this test
// harness can run by writing it to a file and piping stdin to it — i.e. it
// has a shebang. A delivery whose Scripts[key] generator returns something
// else entirely (OpenCode's snippets are TypeScript fragments injected into a
// combined plugin file, not standalone executables) genuinely cannot be
// exercised this way; TestAllRegisteredHooksEmitValidJSON skips those with an
// explicit logged reason rather than silently, so a delivery never quietly
// falls out of coverage the way "claude" alone quietly left "gemini" out.
func isPipeableBashScript(content string) bool {
	return strings.HasPrefix(content, "#!")
}

// TestAllRegisteredHooksEmitValidJSON is the Part D contract test (EPIC.md
// rule 1, gig-1d9d.16): gig-35e2 was one hook emitting empty stdout on a
// no-op path. Nothing before this stopped the *next* one.
//
// Generalized after a #106 follow-up finding: this test originally only ran
// each hook's "claude" script. bashBoth() shares one script body across
// claude and gemini delivery, and Delivery.EventName remaps some event names
// per delivery (Gemini's PostToolUse -> AfterTool, Stop -> AfterAgent). A
// bashBoth() script that hardcoded a literal hookSpecificOutput.hookEventName
// (checkpoint-nudge does exactly this today, just correctly split into two
// delivery-specific closures instead of bashBoth) would emit valid-but-wrong
// JSON under Gemini and nothing would catch it. So this now iterates
// hooks.DeliveryKeys() — every REGISTERED delivery, not a hardcoded
// claude/gemini pair — so the next delivery someone adds is covered on the
// day it registers rather than the day someone remembers to extend this test.
//
// For every hook × every delivery × two contexts (populated / empty), this
// runs a matching input, a no-op input, and garbage, and asserts:
//  1. Rule 1 — parseable JSON on every branch.
//  2. When hookSpecificOutput.hookEventName is present, it equals THAT
//     delivery's EventName(h.Event), not h.Event literally.
func TestAllRegisteredHooksEmitValidJSON(t *testing.T) {
	requireBinaries(t, "bash", "jq")
	stubDir := setupStubs(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	ctxs := []struct {
		name string
		ctx  HookContext
	}{
		{"populated", HookContext{
			TaskID:             "gig-123",
			OrchestratorID:     "jeff-1",
			CheckpointPatterns: []string{"git commit"},
			Persona:            "jenko",
			Repos:              []string{"jeff"},
		}},
		{"empty-context", HookContext{}},
	}

	for _, h := range DefaultRegistry().All() {
		for _, key := range DeliveryKeys() {
			gen, ok := h.Scripts[key]
			if !ok || gen == nil {
				t.Logf("skip %s/%s: no script generator for this delivery (genuinely not exercisable offline)", h.Name, key)
				continue
			}
			d := GetDelivery(key)
			if d == nil {
				t.Fatalf("delivery %q is in DeliveryKeys() but GetDelivery returned nil", key)
			}
			expectedEvent := d.EventName(h)

			for _, c := range ctxs {
				script := gen(c.ctx)
				if script == "" {
					continue // e.g. an OpenCode-only branch returning "" for this ctx
				}
				if !isPipeableBashScript(script) {
					t.Logf("skip %s/%s/%s: content is not a pipeable bash script (no shebang) — this delivery's Scripts[%q] generator produces something else (e.g. injected plugin code), so Rule 1 cannot be checked by running it standalone", h.Name, key, c.name, key)
					continue
				}
				for _, in := range hookContractInputs(h.Event) {
					t.Run(h.Name+"/"+key+"/"+c.name+"/"+in.name, func(t *testing.T) {
						out := runHookScript(t, script, in.in)
						assertValidHookOutput(t, out, expectedEvent)
					})
				}
			}
		}
	}
}

// TestInboxHooksPendingZeroEmitsJSON targets the PENDING=0 no-op branch
// specifically: DefaultRegistry's generic ctx/input combinations in
// TestAllRegisteredHooksEmitValidJSON never make `jeff ... --count` actually
// print "0" (the stub jeff binary only ever writes to stderr), so that branch
// goes unexercised there. Both inbox-replay and orchestrator-inbox had a bare
// `exit 0` on this path — a Rule-1 violation the general contract test alone
// could not catch, exactly the class gig-35e2 came from.
func TestInboxHooksPendingZeroEmitsJSON(t *testing.T) {
	requireBinaries(t, "bash", "jq")
	dir := t.TempDir()
	jeffStub := `#!/bin/bash
for a in "$@"; do
  if [ "$a" = "--count" ]; then
    echo "0"
    exit 0
  fi
done
echo 'stub jeff' >&2
`
	if err := os.WriteFile(filepath.Join(dir, "jeff"), []byte(jeffStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gig"), []byte("#!/bin/bash\necho 'stub gig' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if jqPath, err := exec.LookPath("jq"); err == nil {
		jqBytes, _ := os.ReadFile(jqPath)
		os.WriteFile(filepath.Join(dir, "jq"), jqBytes, 0o755)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("inbox-replay", func(t *testing.T) {
		script := inboxReplayHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
		out := runHookScript(t, script, `{"session_id": "sess-1"}`)
		assertValidHookOutput(t, out, "SessionStart")
	})
	t.Run("orchestrator-inbox", func(t *testing.T) {
		script := orchestratorInboxHook().Scripts["claude"](HookContext{OrchestratorID: "jeff-1"})
		out := runHookScript(t, script, `{"session_id": "sess-1"}`)
		assertValidHookOutput(t, out, "SessionStart")
	})
}

// TestWorkerHeartbeatDebounce is part A(b) of gig-1d9d.16.2: the generated
// script must skip the `jeff crew touch` exec when the sentinel's mtime shows
// a touch happened within the last ~60s, and take it when the sentinel is
// missing or stale. Counts actual execs via a stub `jeff` that appends to a
// log file, run twice in the same directory (so the sentinel persists between
// runs) to exercise both branches.
func TestWorkerHeartbeatDebounce(t *testing.T) {
	requireBinaries(t, "bash")
	dir := t.TempDir()
	logFile := filepath.Join(dir, "touch.log")
	jeffStub := "#!/bin/bash\necho \"$*\" >> " + logFile + "\n"
	if err := os.WriteFile(filepath.Join(dir, "jeff"), []byte(jeffStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	script := workerHeartbeatHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
	scriptPath := filepath.Join(dir, "heartbeat.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	runIn := func(workDir string) {
		t.Helper()
		cmd := exec.Command("bash", scriptPath)
		cmd.Dir = workDir
		cmd.Stdin = strings.NewReader(`{}`)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("script failed: %v\noutput: %s", err, out)
		}
		if !json.Valid(out) {
			t.Errorf("output is not valid JSON: %s", out)
		}
	}

	// No sentinel yet: first call must touch (sentinel missing → stale).
	runIn(dir)
	logLines := func() []string {
		data, _ := os.ReadFile(logFile)
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			return nil
		}
		return lines
	}
	if got := len(logLines()); got != 1 {
		t.Fatalf("execs after first call = %d, want 1 (sentinel missing must touch)", got)
	}

	// Sentinel now fresh (just created): a second call right away must skip.
	runIn(dir)
	if got := len(logLines()); got != 1 {
		t.Fatalf("execs after second (fresh-sentinel) call = %d, want 1 (fresh sentinel must skip)", got)
	}

	// Age the sentinel past the debounce window: the next call must touch again.
	sentinel := filepath.Join(dir, ".heartbeat")
	stale := time.Now().Add(-90 * time.Second)
	if err := os.Chtimes(sentinel, stale, stale); err != nil {
		t.Fatal(err)
	}
	runIn(dir)
	if got := len(logLines()); got != 2 {
		t.Fatalf("execs after stale-sentinel call = %d, want 2 (stale sentinel must touch)", got)
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

// TestWorkerHeartbeatDebounceGNUStat pins the debounce against GNU coreutils
// behaviour, which broke `main` and could not be caught by any macOS-only run.
//
// BSD stat takes `-f %m`; GNU stat's `-f` means --file-system and takes no
// argument, so `%m` is read as a FILENAME. That fails — triggering the `||`
// fallback — but GNU still prints the sentinel's filesystem info to stdout
// first, so LAST ended up as that text concatenated with the fallback's epoch.
// `$((NOW - LAST))` then made bash evaluate "File" as a variable and `set -u`
// aborted the hook:
//
//	heartbeat.sh: line 15: File: unbound variable
//
// A fake GNU-shaped stat on PATH reproduces it on any platform.
func TestWorkerHeartbeatDebounceGNUStat(t *testing.T) {
	requireBinaries(t, "bash")
	dir := t.TempDir()
	logFile := filepath.Join(dir, "touch.log")

	if err := os.WriteFile(filepath.Join(dir, "jeff"),
		[]byte("#!/bin/bash\necho \"$*\" >> "+logFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// GNU-shaped stat: -f prints filesystem info to STDOUT and exits non-zero;
	// -c returns the epoch. This is the combination that broke.
	gnuStat := "#!/bin/bash\n" +
		"if [ \"$1\" = \"-f\" ]; then echo \"  File: \\\"$3\\\"\"; echo \"    ID: 0 Namelen: 255\"; exit 1; fi\n" +
		"if [ \"$1\" = \"-c\" ]; then date +%s; exit 0; fi\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(gnuStat), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	script := workerHeartbeatHook().Scripts["claude"](HookContext{TaskID: "gig-123"})
	scriptPath := filepath.Join(dir, "heartbeat.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	run := func() string {
		t.Helper()
		cmd := exec.Command("bash", scriptPath)
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(`{}`)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("script failed under GNU-shaped stat: %v\noutput: %s", err, out)
		}
		if !json.Valid(out) {
			t.Errorf("output is not valid JSON: %s", out)
		}
		return string(out)
	}

	run() // creates the sentinel
	run() // fresh sentinel: must skip, and must not abort on a non-numeric LAST

	data, _ := os.ReadFile(logFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Errorf("execs = %v, want exactly 1 (the debounce must work under GNU stat too)", lines)
	}
}
