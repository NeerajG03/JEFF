package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

func notifyCmd() *cobra.Command {
	var title, message string

	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Surface a macOS system notification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("jeff notify only supports macOS (current: %s)", runtime.GOOS)
			}

			bin, err := exec.LookPath("terminal-notifier")
			if err != nil {
				return fmt.Errorf("terminal-notifier not found — install with: brew install terminal-notifier")
			}

			if title == "" {
				title = "JEFF"
			}

			return exec.Command(bin, "-title", title, "-message", message).Run()
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Notification headline (default: JEFF)")
	cmd.Flags().StringVar(&message, "message", "", "Notification body (required)")
	_ = cmd.MarkFlagRequired("message")

	return cmd
}
