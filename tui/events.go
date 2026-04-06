package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/NeerajG03/gig"
)

func renderEvents(events []*gig.Event, width int) string {
	header := headerStyle.Render("Activity")

	if len(events) == 0 {
		return panelStyle.Width(width).Render(
			header + "\n" + dimStyle.Render("(no recent activity)"))
	}

	var rows []string
	// Most recent first, max 10.
	maxRows := 10
	start := len(events) - 1
	end := max(0, len(events)-maxRows)

	for i := start; i >= end; i-- {
		ev := events[i]
		desc := describeEvent(ev)
		if desc == "" {
			continue
		}
		age := shortRelTime(ev.Timestamp)
		row := fmt.Sprintf(" %4s  %-11s  %s", age, ev.TaskID, desc)
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return panelStyle.Width(width).Render(
			header + "\n" + dimStyle.Render("(no recent activity)"))
	}

	content := header + "\n" + strings.Join(rows, "\n")
	return panelStyle.Width(width).Render(content)
}

// describeEvent returns a human-readable description of a gig event.
func describeEvent(ev *gig.Event) string {
	switch ev.Type {
	case gig.EventStatusChanged:
		return fmt.Sprintf("status: %s -> %s", ev.OldValue, ev.NewValue)
	case gig.EventCommented:
		if strings.HasPrefix(ev.NewValue, "checkpoint: ") {
			val := strings.TrimPrefix(ev.NewValue, "checkpoint: ")
			return "checkpoint: " + truncate(val, 50)
		}
		return "comment: " + truncate(ev.NewValue, 50)
	case gig.EventAssigned:
		return fmt.Sprintf("assigned to %s", ev.NewValue)
	case gig.EventCreated:
		return "task created"
	case gig.EventUpdated:
		if ev.Field != "" {
			return fmt.Sprintf("updated %s", ev.Field)
		}
		return "updated"
	default:
		return ""
	}
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
