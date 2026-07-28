package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func dashboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Live TUI dashboard for crew management",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			crewStore, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer crewStore.Close()

			gigStore, err := openGigStore(cfg)
			if err != nil {
				return fmt.Errorf("open gig store: %w", err)
			}
			defer gigStore.Close()

			m := tui.New(crewStore, gigStore)
			p := tea.NewProgram(m, tea.WithAltScreen())
			finalModel, err := p.Run()
			if err != nil {
				return err
			}

			// If user pressed 'a' to attach, hand off to tmux.
			if fm, ok := finalModel.(tui.Model); ok && fm.AttachTarget != "" {
				return attachToTmux(fm.AttachTarget)
			}

			return nil
		},
	}
}

// attachToTmux hands the terminal to the tmux session at the given target.
// The target is a full "session:window" string (built by the TUI via
// crew.SessionTarget) so workers hosted in orchestrator-owned sessions
// ("jeff-<suffix>") resolve correctly, not just the shared "jeff" session.
func attachToTmux(target string) error {
	if crew.InsideTmux() {
		// Already in tmux — switch client to the target session + window.
		return execTmuxRun("switch-client", "-t", target)
	}

	// Outside tmux — exec into tmux attach (replaces this process).
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Attaching to tmux target %s...\n", target)
	// Use syscall exec to replace process, keeping the terminal clean.
	return execTmux(tmuxBin, "attach-session", "-t", target)
}

// execTmuxRun looks up tmux and runs the given args via execTmux.
func execTmuxRun(args ...string) error {
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return execTmux(tmuxBin, args...)
}

// execTmux runs tmux and waits (we can't use syscall.Exec on all platforms).
func execTmux(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
