package main

import (
	"fmt"

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

			gigStore, err := openGigStore()
			if err != nil {
				return fmt.Errorf("open gig store: %w", err)
			}
			defer gigStore.Close()

			m := tui.New(crewStore, gigStore)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
