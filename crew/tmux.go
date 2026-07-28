package crew

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// TmuxSessionName is the fixed tmux session name for all crew windows.
const TmuxSessionName = "jeff"

// DashboardWindowName is the name of the first window (index 1) in the jeff session.
const DashboardWindowName = "dashboard"

// EnsureTmux checks that tmux is available in PATH.
func EnsureTmux() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		hint := "install tmux"
		switch runtime.GOOS {
		case "darwin":
			hint = "brew install tmux"
		case "linux":
			hint = "apt/dnf install tmux"
		}
		return fmt.Errorf("tmux not found in PATH — required for crew management (install: %s)", hint)
	}
	return nil
}

// EnsureSession ensures the "jeff" tmux session exists.
// Creates a detached session with a "dashboard" window at index 1 (base-index).
// No-op if already present.
func EnsureSession() error {
	if hasSession(TmuxSessionName) {
		return nil
	}
	return tmuxRun("new-session", "-d", "-s", TmuxSessionName, "-n", DashboardWindowName, "-x", "200", "-y", "50")
}

// SanitizeWindowName replaces dots with hyphens in tmux window names.
// tmux interprets dots in target specs as session.window.pane separators,
// so task IDs like "gig-45c2.2" would be parsed as session "jeff:gig-45c2" window "2".
func SanitizeWindowName(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

// CreateWindow creates a new window in the jeff tmux session.
// Returns the tmux target string "jeff:<name>".
// The window name is sanitized (dots replaced with hyphens) to avoid
// tmux interpreting dots as session.window.pane separators.
func CreateWindow(name, dir string) (string, error) {
	name = SanitizeWindowName(name)
	target := TmuxSessionName + ":" + name
	err := tmuxRun("new-window", "-d", "-a", "-t", TmuxSessionName, "-n", name, "-c", dir)
	if err != nil {
		return "", fmt.Errorf("create tmux window %q: %w", name, err)
	}
	keepPaneOnExit(target)
	return target, nil
}

// SendText sends text to a tmux pane WITHOUT pressing Enter.
// Uses -l (literal) so embedded newlines and special characters are
// pasted as-is rather than interpreted as tmux key names.
func SendText(target, text string) error {
	return tmuxRun("send-keys", "-t", target, "-l", text)
}

// SendEnter sends the Enter key to a tmux pane.
// This is the second half of sending a command.
func SendEnter(target string) error {
	return tmuxRun("send-keys", "-t", target, "Enter")
}

// SendCommand sends text followed by Enter to a tmux pane.
// The paste and Enter are sent as two separate tmux invocations
// with a small delay between them so the pane has time to process
// the pasted text before receiving the Enter keypress. Without this
// delay, tmux can swallow the Enter on large pastes.
func SendCommand(target, command string) error {
	return SendCommandWithDelay(target, command, 100*time.Millisecond)
}

// SendCommandWithDelay is like SendCommand but lets the caller control the
// paste-to-Enter delay. Use this when the default 100 ms is insufficient —
// for example, Gemini CLI's Ink/React TUI processes input asynchronously and
// drops the Enter if it arrives before the pasted text is committed to the
// input buffer.
func SendCommandWithDelay(target, command string, delay time.Duration) error {
	if err := tmuxRun("send-keys", "-t", target, "-l", command); err != nil {
		return err
	}
	time.Sleep(delay)
	return tmuxRun("send-keys", "-t", target, "Enter")
}

// SendCommandViaBuffer writes the command to a temporary tmux buffer and pastes
// it into the target pane using bracketed paste mode (-p). The buffer is deleted
// (-d) immediately after pasting to prevent leaks and cross-worker race conditions.
func SendCommandViaBuffer(target, command string, pasteDelay time.Duration) error {
	bufName := fmt.Sprintf("jeff-send-%d", os.Getpid())
	cmd := exec.Command("tmux", "load-buffer", "-b", bufName, "-")
	cmd.Stdin = strings.NewReader(command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux load-buffer: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	if err := tmuxRun("paste-buffer", "-b", bufName, "-t", target, "-p", "-d"); err != nil {
		return err
	}
	time.Sleep(pasteDelay)
	return tmuxRun("send-keys", "-t", target, "Enter")
}

// SendInterrupt sends C-c to a tmux pane.
func SendInterrupt(target string) error {
	return tmuxRun("send-keys", "-t", target, "C-c")
}

// CapturePane captures the visible content of a tmux pane.
// lines specifies how many lines of scrollback to include (from bottom).
func CapturePane(target string, lines int) (string, error) {
	start := fmt.Sprintf("-%d", lines)
	out, err := tmuxOutput("capture-pane", "-t", target, "-p", "-S", start)
	if err != nil {
		return "", fmt.Errorf("capture pane %q: %w", target, err)
	}
	return strings.TrimRight(out, "\n"), nil
}

// SessionTarget builds a sanitized tmux target string "session:window".
// Always use this instead of manual string concatenation to ensure
// dots in task IDs are replaced with hyphens.
func SessionTarget(tmuxSession, windowName string) string {
	return tmuxSession + ":" + SanitizeWindowName(windowName)
}

// SelectWindow switches to the given tmux window.
func SelectWindow(target string) error {
	return tmuxRun("select-window", "-t", target)
}

// InsideTmux returns true if the current process is inside a tmux session.
func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// InsideJeffManagedSession reports whether the current process is running inside
// a jeff-managed tmux session — the shared default "jeff" session or an
// orchestrator "jeff-<suffix>" session. Callers use this to fail loud when
// orchestrator detection comes back empty inside such a session, rather than
// silently spawning a standalone worker (gig-9c92 Option B).
func InsideJeffManagedSession() bool {
	if os.Getenv("TMUX") == "" {
		return false
	}
	out, err := tmuxOutput("display-message", "-t", os.Getenv("TMUX_PANE"), "-p", "#{session_name}")
	if err != nil {
		return false
	}
	name := strings.TrimSpace(out)
	return name == TmuxSessionName || strings.HasPrefix(name, TmuxSessionName+"-")
}

// KillWindow kills a specific tmux window.
func KillWindow(target string) error {
	return tmuxRun("kill-window", "-t", target)
}

// HasWindow checks if a window exists in the jeff session.
// The name is sanitized before comparison to handle task IDs with dots.
func HasWindow(name string) bool {
	return HasWindowInSession(TmuxSessionName, name)
}

// HasWindowInSession checks if a window exists in a specific tmux session.
func HasWindowInSession(sessionName, name string) bool {
	name = SanitizeWindowName(name)
	windows, err := ListWindowsInSession(sessionName)
	if err != nil {
		return false
	}
	for _, w := range windows {
		if w == name {
			return true
		}
	}
	return false
}

// ListWindows returns all window names in the jeff session.
func ListWindows() ([]string, error) {
	return ListWindowsInSession(TmuxSessionName)
}

// ListWindowsInSession returns all window names in a specific tmux session.
func ListWindowsInSession(sessionName string) ([]string, error) {
	out, err := tmuxOutput("list-windows", "-t", sessionName, "-F", "#{window_name}")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// WindowPID returns the PID of the foreground process in a tmux pane.
func WindowPID(target string) (int, error) {
	out, err := tmuxOutput("display-message", "-t", target, "-p", "#{pane_pid}")
	if err != nil {
		return 0, fmt.Errorf("get pane PID for %q: %w", target, err)
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &pid); err != nil {
		return 0, fmt.Errorf("parse PID %q: %w", out, err)
	}
	return pid, nil
}

// CreateSession creates a new tmux session with the given name and initial window.
// Returns the target "session:window".
func CreateSession(sessionName, windowName, dir string) (string, error) {
	if hasSession(sessionName) {
		return "", fmt.Errorf("tmux session %q already exists", sessionName)
	}
	err := tmuxRun("new-session", "-d", "-s", sessionName, "-n", windowName, "-c", dir, "-x", "200", "-y", "50")
	if err != nil {
		return "", fmt.Errorf("create tmux session %q: %w", sessionName, err)
	}
	return sessionName + ":" + windowName, nil
}

// CreateWindowInSession creates a new window in an arbitrary tmux session.
// Returns the target "session:window".
func CreateWindowInSession(sessionName, windowName, dir string) (string, error) {
	windowName = SanitizeWindowName(windowName)
	target := sessionName + ":" + windowName
	err := tmuxRun("new-window", "-d", "-a", "-t", sessionName, "-n", windowName, "-c", dir)
	if err != nil {
		return "", fmt.Errorf("create tmux window %q in %q: %w", windowName, sessionName, err)
	}
	keepPaneOnExit(target)
	return target, nil
}

// PaneID returns the unique pane ID (e.g. %42) for a target.
func PaneID(target string) (string, error) {
	out, err := tmuxOutput("display-message", "-t", target, "-p", "#{pane_id}")
	if err != nil {
		return "", fmt.Errorf("get pane ID for %q: %w", target, err)
	}
	return strings.TrimSpace(out), nil
}

// HasSession checks if a tmux session exists.
func HasSession(name string) bool {
	return hasSession(name)
}

// PaneIsDead reports whether the pane at the given target has exited.
// With remain-on-exit on, the window persists but the pane shows a dead
// status indicator instead of being destroyed.
//
// It is tri-state: a probe error (target doesn't exist, tmux busy, a
// transient failure) is NOT death — it returns (false, err) so callers can
// treat the result as "unknown, not dead". Only a successful probe reporting
// #{pane_dead}=="1" yields (true, nil). Never treat a probe error as death;
// doing so false-fails live workers on a single transient tmux hiccup.
func PaneIsDead(target string) (bool, error) {
	out, err := tmuxOutput("display-message", "-t", target, "-p", "#{pane_dead}")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

// AttachToSession attaches to an arbitrary tmux session (not just "jeff").
// Uses an interactive exec so tmux can take over the terminal's TTY.
func AttachToSession(sessionName, windowName string) error {
	target := sessionName
	if windowName != "" {
		target = sessionName + ":" + SanitizeWindowName(windowName)
	}
	var args []string
	if InsideTmux() {
		args = []string{"switch-client", "-t", target}
	} else {
		args = []string{"attach-session", "-t", target}
	}
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// TmuxWindow represents a window in a tmux session.
type TmuxWindow struct {
	Session string
	Window  string
}

// WindowInfo describes a tmux window with the state cleanup decisions need:
// a stable window ID (safe kill target even when the display name has been
// renamed or contains target-metacharacters like dots) and whether the
// window's active pane has exited (a remain-on-exit leftover) or is live.
type WindowInfo struct {
	Session  string
	Window   string
	WindowID string // tmux window id, e.g. "@42"
	PaneDead bool   // active pane exited (remain-on-exit keeps the window)
}

// listWindowInfoFormat orders window_name last so the two fixed-position
// fields can be split off safely (window ids and pane_dead never contain
// spaces; names may). Tabs are not usable as separators — tmux sanitizes
// control characters in list-command output to "_".
const listWindowInfoFormat = "#{window_id} #{pane_dead} #{window_name}"

// ListSessionWindowInfo returns per-window info (id, active-pane liveness,
// name) for a tmux session.
func ListSessionWindowInfo(sessionName string) ([]WindowInfo, error) {
	out, err := tmuxOutput("list-windows", "-t", sessionName, "-F", listWindowInfoFormat)
	if err != nil {
		return nil, err
	}
	var result []WindowInfo
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 || !strings.HasPrefix(parts[0], "@") {
			continue
		}
		result = append(result, WindowInfo{
			Session:  sessionName,
			Window:   parts[2],
			WindowID: parts[0],
			PaneDead: strings.TrimSpace(parts[1]) == "1",
		})
	}
	return result, nil
}

// ListSessionWindows returns all window names in the given tmux session.
func ListSessionWindows(sessionName string) ([]string, error) {
	out, err := tmuxOutput("list-windows", "-t", sessionName, "-F", "#{window_name}")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ListAllJeffSessions returns the names of all tmux sessions matching "jeff" or "jeff-N".
func ListAllJeffSessions() ([]string, error) {
	out, err := tmuxOutput("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, nil // tmux not running or no sessions
	}
	var result []string
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if name == TmuxSessionName || strings.HasPrefix(name, TmuxSessionName+"-") {
			result = append(result, name)
		}
	}
	return result, nil
}

// ListAllJeffWindows returns all windows across jeff and jeff-N sessions.
func ListAllJeffWindows() ([]TmuxWindow, error) {
	sessions, err := ListAllJeffSessions()
	if err != nil {
		return nil, err
	}
	var result []TmuxWindow
	for _, sess := range sessions {
		windows, err := ListSessionWindows(sess)
		if err != nil {
			continue
		}
		for _, w := range windows {
			result = append(result, TmuxWindow{Session: sess, Window: w})
		}
	}
	return result, nil
}

// HasWindowInSession is defined earlier in this file (line ~139).
// The worker's cleanup code can use it directly.

// KillSession kills an entire tmux session.
func KillSession(sessionName string) error {
	return tmuxRun("kill-session", "-t", sessionName)
}

// --- internal helpers ---

// keepPaneOnExit sets the tmux `remain-on-exit` window option so the pane is
// NOT destroyed when its foreground process (the agent CLI) exits — whether the
// agent finishes its turn, crashes, or is interrupted on worker-stop. The pane
// stays in a "dead" state showing its final output so it can be inspected
// instead of silently vanishing. Best-effort: a failure here must not abort
// window creation, so the error is intentionally ignored.
func keepPaneOnExit(target string) {
	_ = tmuxRun("set-option", "-w", "-t", target, "remain-on-exit", "on")
}

func hasSession(name string) bool {
	out, err := tmuxOutput("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == name {
			return true
		}
	}
	return false
}

func tmuxRun(args ...string) error {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %w (output: %s)", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

func tmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return string(out), nil
}
