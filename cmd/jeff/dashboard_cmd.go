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

// attachToTmux hands the terminal to the jeff tmux session at the given window.
func attachToTmux(windowName string) error {
	target := crew.TmuxSessionName + ":" + windowName

	if crew.InsideTmux() {
		// Already in tmux — switch client to the jeff session + window.
		return crew.AttachSession(windowName)
	}

	// Outside tmux — exec into tmux attach (replaces this process).
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Attaching to tmux window %s...\n", windowName)
	// Use syscall exec to replace process, keeping the terminal clean.
	return execTmux(tmuxBin, "attach-session", "-t", target)
}

// execTmux runs tmux and waits (we can't use syscall.Exec on all platforms).
func execTmux(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
