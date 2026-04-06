package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/NeerajG03/gig"
)

func renderEvents(events []*gig.Event, width int) string {
	header := headerStyle.Render("Events")

	if len(events) == 0 {
		return panelStyle.Width(width).Render(header + "\n" + dimStyle.Render("(no recent events)"))
	}

	var rows []string
	// Show most recent first, max 10.
	limit := len(events)
	if limit > 10 {
		limit = 10
	}
	for i := len(events) - 1; i >= len(events)-limit; i-- {
		ev := events[i]
		age := shortRelTime(ev.Timestamp)
		summary := ev.NewValue
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		if ev.Type == gig.EventStatusChanged {
			summary = ev.OldValue + " -> " + ev.NewValue
		}
		row := fmt.Sprintf("%-6s %-12s %-14s %s", age, ev.TaskID, ev.Type, summary)
		rows = append(rows, row)
	}

	content := header + "\n" + strings.Join(rows, "\n")
	return panelStyle.Width(width).Render(content)
}

func shortRelTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
