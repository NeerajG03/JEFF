// Package tui provides a Bubbletea-based dashboard for crew management.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors.
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	dimmed    = lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"}
	danger    = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	warning   = lipgloss.AdaptiveColor{Light: "#F1C40F", Dark: "#F1C40F"}

	// Base styles.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(special)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#DDDDDD"})

	dimStyle = lipgloss.NewStyle().
			Foreground(dimmed)

	statusRunning  = lipgloss.NewStyle().Foreground(special).Bold(true).Render("●")
	statusStarting = lipgloss.NewStyle().Foreground(warning).Render("◉")
	statusDone     = lipgloss.NewStyle().Foreground(dimmed).Render("○")
	statusFailed   = lipgloss.NewStyle().Foreground(danger).Bold(true).Render("✕")
	statusStopped  = lipgloss.NewStyle().Foreground(warning).Render("■")

	// Panel styles.
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(dimmed)

	labelStyle = lipgloss.NewStyle().
			Foreground(dimmed).
			Width(14)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#DDDDDD"})
)

func statusIcon(status string) string {
	switch status {
	case "running":
		return statusRunning
	case "starting":
		return statusStarting
	case "done":
		return statusDone
	case "failed":
		return statusFailed
	case "stopped":
		return statusStopped
	default:
		return "?"
	}
}
