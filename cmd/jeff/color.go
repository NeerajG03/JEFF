package main

import (
	"fmt"
	"os"
	"regexp"

	"golang.org/x/term"

	"github.com/NeerajG03/gig"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ANSI color codes.
const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cCyan    = "\033[36m"
)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func colorize(color, s string) string {
	if !colorEnabled() {
		return s
	}
	return color + s + cReset
}

func colorStatus(s gig.Status) string {
	icon := statusIconFor(s)
	if !colorEnabled() {
		return icon
	}
	switch s {
	case gig.StatusInProgress:
		return colorize(cBlue+cBold, icon)
	case gig.StatusBlocked:
		return colorize(cRed+cBold, icon)
	case gig.StatusDeferred:
		return colorize(cYellow, icon)
	case gig.StatusClosed:
		return colorize(cGreen, icon)
	case gig.StatusCancelled:
		return colorize(cMagenta, icon)
	default:
		return icon
	}
}

func colorPriority(p gig.Priority) string {
	label := fmt.Sprintf("P%d", p)
	if !colorEnabled() {
		return label
	}
	switch p {
	case gig.P0:
		return colorize(cRed+cBold, label)
	case gig.P1:
		return colorize(cYellow+cBold, label)
	case gig.P3, gig.P4:
		return colorize(cDim, label)
	default:
		return label
	}
}

// visibleLen returns the display width of s, ignoring ANSI escape sequences.
func visibleLen(s string) int {
	return len([]rune(ansiRE.ReplaceAllString(s, "")))
}

// crewLegend prints a status icon legend to stdout.
func crewLegend() {
	fmt.Fprintf(os.Stdout, "\nLegend: %s  %s  %s  %s  %s\n",
		colorize(cGreen+cBold, "● running"),
		colorize(cYellow, "◉ starting"),
		colorize(cDim, "○ done"),
		colorize(cRed+cBold, "✕ failed"),
		colorize(cYellow, "■ stopped"),
	)
}
