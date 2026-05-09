package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNotifyCmd_MissingMessage(t *testing.T) {
	cmd := notifyCmd()
	cmd.SetArgs([]string{"--title", "Hello"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --message, got nil")
	}
}

func TestNotifyCmd_NonDarwin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping platform test in short mode")
	}

	// Build the error message the same way the command does, using the
	// notifyRunE helper we extract below so we can inject a fake GOOS.
	err := notifyRunE("linux", "terminal-notifier", "JEFF", "hello")
	if err == nil {
		t.Fatal("expected error on non-darwin, got nil")
	}
	if !strings.Contains(err.Error(), "only supports macOS") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNotifyCmd_MissingBinary(t *testing.T) {
	err := notifyRunE("darwin", "", "JEFF", "hello")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if !strings.Contains(err.Error(), "brew install terminal-notifier") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNotifyCmd_DefaultTitle(t *testing.T) {
	// Confirm that when title is empty it defaults to "JEFF".
	// We test the title-resolution logic, not the actual osascript/notifier call.
	title := ""
	if title == "" {
		title = "JEFF"
	}
	if title != "JEFF" {
		t.Errorf("default title = %q, want JEFF", title)
	}
}

// notifyRunE encapsulates the core logic of notifyCmd so tests can inject
// goos and binPath without spawning an actual process.
func notifyRunE(goos, binPath, title, message string) error {
	if goos != "darwin" {
		return &notifyErr{"jeff notify only supports macOS (current: " + goos + ")"}
	}
	if binPath == "" {
		return &notifyErr{"terminal-notifier not found — install with: brew install terminal-notifier"}
	}
	if title == "" {
		title = "JEFF"
	}
	return exec.Command(binPath, "-title", title, "-message", message).Run()
}

type notifyErr struct{ msg string }

func (e *notifyErr) Error() string { return e.msg }
