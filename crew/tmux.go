package crew

import (
	"fmt"
	"os"
	"os/exec"
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
		return fmt.Errorf("tmux not found in PATH — required for crew management (install: brew install tmux)")
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
	err := tmuxRun("new-window", "-a", "-t", TmuxSessionName, "-n", name, "-c", dir)
	if err != nil {
		return "", fmt.Errorf("create tmux window %q: %w", name, err)
	}
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
// Convenience wrapper: SendText + small delay + SendEnter.
func SendCommand(target, command string) error {
	if err := SendText(target, command); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return SendEnter(target)
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

// AttachSession attaches to the jeff tmux session and selects a window.
// If already inside tmux, uses switch-client. Otherwise uses attach-session.
func AttachSession(windowName string) error {
	windowName = SanitizeWindowName(windowName)
	target := TmuxSessionName + ":" + windowName
	if InsideTmux() {
		// Already in tmux — switch to the jeff session + window.
		return tmuxRun("switch-client", "-t", target)
	}
	// Outside tmux — attach to session and select window.
	return tmuxRun("attach-session", "-t", target)
}

// InsideTmux returns true if the current process is inside a tmux session.
func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// KillWindow kills a specific tmux window.
func KillWindow(target string) error {
	return tmuxRun("kill-window", "-t", target)
}

// HasWindow checks if a window exists in the jeff session.
// The name is sanitized before comparison to handle task IDs with dots.
func HasWindow(name string) bool {
	name = SanitizeWindowName(name)
	windows, err := ListWindows()
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
	out, err := tmuxOutput("list-windows", "-t", TmuxSessionName, "-F", "#{window_name}")
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

// --- internal helpers ---

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
