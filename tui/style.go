// Package tui provides a Bubbletea-based dashboard for crew management.
package tui

import "github.com/charmbracelet/lipgloss"

// Gruvbox Dark palette.
var (
	gruvBg0    = lipgloss.Color("#282828")
	gruvBg1    = lipgloss.Color("#3c3836")
	gruvBg2    = lipgloss.Color("#504945")
	gruvFg0    = lipgloss.Color("#fbf1c7")
	gruvFg1    = lipgloss.Color("#ebdbb2")
	gruvFg2    = lipgloss.Color("#d5c4a1")
	gruvGray   = lipgloss.Color("#928374")
	gruvRed    = lipgloss.Color("#fb4934")
	gruvGreen  = lipgloss.Color("#b8bb26")
	gruvYellow = lipgloss.Color("#fabd2f")
	gruvBlue   = lipgloss.Color("#83a598")
	gruvPurple = lipgloss.Color("#d3869b")
	gruvAqua   = lipgloss.Color("#8ec07c")
	gruvOrange = lipgloss.Color("#fe8019")

	// Base styles.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(gruvYellow)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(gruvAqua)

	normalStyle = lipgloss.NewStyle().
			Foreground(gruvFg1)

	dimStyle = lipgloss.NewStyle().
			Foreground(gruvGray)

	statusRunning  = lipgloss.NewStyle().Foreground(gruvGreen).Bold(true).Render("●")
	statusStarting = lipgloss.NewStyle().Foreground(gruvYellow).Render("◉")
	statusDone     = lipgloss.NewStyle().Foreground(gruvGray).Render("○")
	statusFailed   = lipgloss.NewStyle().Foreground(gruvRed).Bold(true).Render("✕")
	statusStopped  = lipgloss.NewStyle().Foreground(gruvOrange).Render("■")

	// Panel styles.
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gruvBg2).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(gruvBlue)

	helpStyle = lipgloss.NewStyle().
			Foreground(gruvGray)

	labelStyle = lipgloss.NewStyle().
			Foreground(gruvGray).
			Width(14)

	valueStyle = lipgloss.NewStyle().
			Foreground(gruvFg1)

	lipglossWarning = lipgloss.NewStyle().
			Foreground(gruvYellow).
			Bold(true)

	// Layout helpers.
	lipglossLeft  = lipgloss.NewStyle()
	lipglossRight = lipgloss.NewStyle()
)

func lipglossJoinH(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

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
