package tui

import (
	"fmt"
	"strings"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/gig"
)

func renderSessionList(sessions []*crew.Session, selected int, gigStore *gig.Store, titles map[string]string, width int) string {
	if len(sessions) == 0 {
		return panelStyle.Width(width).Render(
			dimStyle.Render("No sessions. Start one: jeff crew start <gig-id> --persona jenko"))
	}

	running := 0
	for _, s := range sessions {
		if s.Status == "running" || s.Status == "starting" {
			running++
		}
	}

	header := headerStyle.Render(fmt.Sprintf("Workers (%d active, %d total)", running, len(sessions)))

	// Column header.
	colHeader := dimStyle.Render(fmt.Sprintf("   %-10s %-8s %-8s %-30s %-30s %5s",
		"TASK", "PERSONA", "STATUS", "TITLE", "CHECKPOINT", "AGE"))

	var rows []string
	rows = append(rows, colHeader)
	for i, sess := range sessions {
		icon := statusIcon(sess.Status)
		persona := sess.Persona
		if persona == "" {
			persona = "-"
		}

		title := titles[sess.TaskID]
		if title == "" {
			title = "-"
		}
		title = truncate(title, 28)

		ckpt := ""
		if gigStore != nil {
			if cp, err := gigStore.LatestCheckpoint(sess.TaskID); err == nil && cp != nil {
				ckpt = truncate(cp.Done, 28)
			}
		}
		if ckpt == "" {
			ckpt = "-"
		}

		age := shortRelTime(sess.StartedAt)

		line := fmt.Sprintf("  %s %-10s %-8s %-8s %-30s %-30s %5s",
			icon, sess.TaskID, persona, sess.Status, title, ckpt, age)

		if i == selected {
			line = selectedStyle.Render("> " + line[2:])
		}

		rows = append(rows, line)
	}

	content := header + "\n" + strings.Join(rows, "\n")
	return panelStyle.Width(width).Render(content)
}

func renderDetail(sess *crew.Session, taskTitle string, gigStore *gig.Store, crewStore *crew.Store, paneOutput string, width int) string {
	if sess == nil {
		return ""
	}

	// Header.
	header := sess.TaskID
	if taskTitle != "" {
		header += " — " + taskTitle
	}
	headerLine := headerStyle.Render(header)

	// Metadata fields.
	var meta []string
	meta = append(meta, renderField("Persona", sess.Persona))
	if len(sess.Repos) > 0 {
		meta = append(meta, renderField("Repos", strings.Join(sess.Repos, ", ")))
	}
	meta = append(meta, renderField("Status", sess.Status))
	meta = append(meta, renderField("Started", shortRelTime(sess.StartedAt)+" ago"))

	if gigStore != nil {
		if cp, err := gigStore.LatestCheckpoint(sess.TaskID); err == nil && cp != nil {
			meta = append(meta, renderField("Done", truncate(cp.Done, 60)))
			if cp.Next != "" {
				meta = append(meta, renderField("Next", truncate(cp.Next, 60)))
			}
			if cp.Decisions != "" {
				meta = append(meta, renderField("Decisions", truncate(cp.Decisions, 60)))
			}
		}
	}

	if crewStore != nil {
		count, _ := crewStore.PendingCount(sess.TaskID, "to_worker")
		if count > 0 {
			meta = append(meta, renderField("Inbox",
				lipglossWarning.Render(fmt.Sprintf("%d pending", count))))
		}

		// Recent messages (most recent 3).
		msgs, _ := crewStore.RecentMessages(sess.TaskID, 3)
		if len(msgs) > 0 {
			meta = append(meta, "")
			meta = append(meta, dimStyle.Render("Recent messages:"))
			for _, msg := range msgs {
				dir := ">"
				if msg.Direction == "to_orchestrator" {
					dir = "<"
				}
				ack := ""
				if msg.AckedAt != nil {
					ack = " " + dimStyle.Render("[acked]")
				}
				meta = append(meta, fmt.Sprintf("  %s [%s] %s%s",
					dir, msg.Type, truncate(msg.Content, 50), ack))
			}
		}
	}

	metaStr := strings.Join(meta, "\n")

	// Pane output (only when user pressed 'c').
	if paneOutput != "" {
		paneLines := strings.Split(paneOutput, "\n")
		for len(paneLines) > 0 && strings.TrimSpace(paneLines[0]) == "" {
			paneLines = paneLines[1:]
		}
		if len(paneLines) > 12 {
			paneLines = paneLines[len(paneLines)-12:]
		}

		paneRendered := dimStyle.Render("Terminal (c to refresh):") + "\n"
		for _, l := range paneLines {
			paneRendered += dimStyle.Render(" "+truncate(l, width/2-6)) + "\n"
		}

		if width > 90 {
			leftW := width/2 - 3
			rightW := width/2 - 3
			left := lipglossLeft.Width(leftW).Render(metaStr)
			right := lipglossRight.Width(rightW).Render(paneRendered)
			body := lipglossJoinH(left, right)
			return panelStyle.Width(width).Render(headerLine + "\n" + body)
		}
		return panelStyle.Width(width).Render(headerLine + "\n" + metaStr + "\n\n" + paneRendered)
	}

	return panelStyle.Width(width).Render(headerLine + "\n" + metaStr)
}

func renderField(label, value string) string {
	if value == "" {
		value = "-"
	}
	return labelStyle.Render(label+":") + " " + valueStyle.Render(value)
}
