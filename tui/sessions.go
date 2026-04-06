package tui

import (
	"fmt"
	"strings"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/charmbracelet/lipgloss"
	"github.com/NeerajG03/gig"
)

func renderSessionList(sessions []*crew.Session, selected int, gigStore *gig.Store, width int) string {
	if len(sessions) == 0 {
		return panelStyle.Width(width).Render(dimStyle.Render("(no active sessions)"))
	}

	count := 0
	for _, s := range sessions {
		if s.Status == "running" || s.Status == "starting" {
			count++
		}
	}

	header := headerStyle.Render(fmt.Sprintf("Workers (%d active)", count))

	var rows []string
	for i, sess := range sessions {
		icon := statusIcon(sess.Status)
		persona := sess.Persona
		if persona == "" {
			persona = "-"
		}

		ckpt := ""
		if gigStore != nil {
			if cp, err := gigStore.LatestCheckpoint(sess.TaskID); err == nil && cp != nil {
				summary := cp.Done
				if len(summary) > 35 {
					summary = summary[:32] + "..."
				}
				ckpt = fmt.Sprintf("%q", summary)
			}
		}
		if ckpt == "" {
			ckpt = dimStyle.Render("(no checkpoint)")
		}

		age := shortRelTime(sess.StartedAt)

		line := fmt.Sprintf(" %s %-11s %-8s %s  %-38s %s",
			icon, sess.TaskID, persona, sess.Status, ckpt, age)

		if i == selected {
			line = selectedStyle.Render("> " + line[1:])
		}

		rows = append(rows, line)
	}

	content := header + "\n" + strings.Join(rows, "\n")
	return panelStyle.Width(width).Render(content)
}

func renderDetail(sess *crew.Session, gigStore *gig.Store, crewStore *crew.Store, paneOutput string, width int) string {
	if sess == nil {
		return ""
	}

	title := headerStyle.Render(fmt.Sprintf("%s", sess.TaskID))

	var lines []string
	lines = append(lines, renderField("Persona", sess.Persona))
	if len(sess.Repos) > 0 {
		lines = append(lines, renderField("Repos", strings.Join(sess.Repos, ", ")))
	}
	lines = append(lines, renderField("Status", sess.Status))
	lines = append(lines, renderField("Started", shortRelTime(sess.StartedAt)+" ago"))

	if gigStore != nil {
		if cp, err := gigStore.LatestCheckpoint(sess.TaskID); err == nil && cp != nil {
			lines = append(lines, renderField("Checkpoint", fmt.Sprintf("%q (%s ago)", cp.Done, shortRelTime(cp.CreatedAt))))
			if cp.Next != "" {
				lines = append(lines, renderField("Next", cp.Next))
			}
		}
	}

	if crewStore != nil {
		count, _ := crewStore.PendingCount(sess.TaskID, "to_worker")
		if count > 0 {
			lines = append(lines, renderField("Inbox", fmt.Sprintf("%d pending", count)))
		}
	}

	// Split: left info, right pane output.
	info := strings.Join(lines, "\n")

	if paneOutput != "" {
		paneLines := strings.Split(paneOutput, "\n")
		// Trim to last 8 lines.
		if len(paneLines) > 8 {
			paneLines = paneLines[len(paneLines)-8:]
		}
		paneRendered := dimStyle.Render("Pane:") + "\n"
		for _, l := range paneLines {
			paneRendered += dimStyle.Render("  > "+l) + "\n"
		}

		// Side by side if width allows.
		if width > 80 {
			left := lipgloss.NewStyle().Width(width/2 - 2).Render(info)
			right := lipgloss.NewStyle().Width(width/2 - 2).Render(paneRendered)
			content := title + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right)
			return panelStyle.Width(width).Render(content)
		}
		content := title + "\n" + info + "\n\n" + paneRendered
		return panelStyle.Width(width).Render(content)
	}

	content := title + "\n" + info
	return panelStyle.Width(width).Render(content)
}

func renderField(label, value string) string {
	if value == "" {
		value = "-"
	}
	return labelStyle.Render(label+":") + " " + valueStyle.Render(value)
}
