package tui

import (
	"fmt"
	"strings"

	"github.com/NeerajG03/gig"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Gig list rendering ---

func (m Model) renderGigsList(width int) string {
	items := m.gigsListItems()

	if len(items) == 0 {
		return panelStyle.Width(width).Render(
			headerStyle.Render("Tasks") + "\n" +
				dimStyle.Render("No ready or in_progress tasks. Press n to create one."))
	}

	readyCount := len(m.readyTasks)
	ipCount := len(m.allTasks)
	header := headerStyle.Render(fmt.Sprintf("Tasks (%d ready, %d in progress)", readyCount, ipCount))

	colHeader := dimStyle.Render(fmt.Sprintf("   %-10s %-4s %-8s %-8s %s",
		"ID", "PRI", "TYPE", "STATUS", "TITLE"))

	var rows []string
	rows = append(rows, colHeader)

	for i, task := range items {
		pri := fmt.Sprintf("P%d", task.Priority)
		priStyled := pri
		switch task.Priority {
		case gig.P0:
			priStyled = lipglossRed.Render(pri)
		case gig.P1:
			priStyled = lipglossOrange.Render(pri)
		}

		statusStyled := string(task.Status)
		switch task.Status {
		case gig.StatusOpen:
			statusStyled = lipglossGreen.Render("ready")
		case gig.StatusInProgress:
			statusStyled = lipglossBlue.Render("active")
		}

		title := truncate(task.Title, 50)

		line := fmt.Sprintf("  %-10s %-4s %-8s %-8s %s",
			task.ID, priStyled, task.Type, statusStyled, title)

		if i == m.gigsSel {
			line = selectedStyle.Render("> " + line[2:])
		}

		rows = append(rows, line)
	}

	content := header + "\n" + strings.Join(rows, "\n")
	return panelStyle.Width(width).Render(content)
}

// --- Gig detail rendering ---

func (m Model) renderGigDetail(width int) string {
	items := m.gigsListItems()
	if m.gigsSel >= len(items) || len(items) == 0 {
		return ""
	}

	task := items[m.gigsSel]
	header := headerStyle.Render(fmt.Sprintf("%s — %s", task.ID, task.Title))

	var meta []string
	meta = append(meta, renderField("Priority", fmt.Sprintf("P%d", task.Priority)))
	meta = append(meta, renderField("Type", string(task.Type)))
	meta = append(meta, renderField("Status", string(task.Status)))
	if task.Assignee != "" {
		meta = append(meta, renderField("Assignee", task.Assignee))
	}
	if len(task.Labels) > 0 {
		meta = append(meta, renderField("Labels", strings.Join(task.Labels, ", ")))
	}
	if task.Description != "" {
		desc := task.Description
		// Show more description, wrap at width.
		if len(desc) > width-20 {
			desc = truncate(desc, width-20)
		}
		meta = append(meta, "")
		meta = append(meta, dimStyle.Render("Description:"))
		meta = append(meta, "  "+desc)
	}

	// Show latest checkpoint if any.
	if m.gigStore != nil {
		if cp, err := m.gigStore.LatestCheckpoint(task.ID); err == nil && cp != nil {
			meta = append(meta, "")
			meta = append(meta, dimStyle.Render("Last checkpoint:"))
			meta = append(meta, "  "+truncate(cp.Done, 80))
			if cp.Next != "" {
				meta = append(meta, "  Next: "+truncate(cp.Next, 60))
			}
		}
	}

	content := header + "\n" + strings.Join(meta, "\n")
	return panelStyle.Width(width).Render(content)
}

// --- Task form (create + edit) ---

type taskFormModel struct {
	fields      []textinput.Model
	activeField int
	labels      []string
	editing     string // task ID if editing, empty if creating
}

func newCreateForm() taskFormModel {
	return newTaskForm("", "", "", "task", "2")
}

func newEditForm(task *gig.Task) taskFormModel {
	return newTaskForm(
		task.ID,
		task.Title,
		task.Description,
		string(task.Type),
		fmt.Sprintf("%d", task.Priority),
	)
}

func newTaskForm(editID, title, desc, taskType, priority string) taskFormModel {
	titleInput := textinput.New()
	titleInput.Placeholder = "Task title (required)"
	titleInput.CharLimit = 200
	titleInput.Width = 60
	titleInput.SetValue(title)

	descInput := textinput.New()
	descInput.Placeholder = "Description (optional)"
	descInput.CharLimit = 500
	descInput.Width = 60
	descInput.SetValue(desc)

	typeInput := textinput.New()
	typeInput.Placeholder = "task, bug, feature, epic, chore"
	typeInput.CharLimit = 20
	typeInput.Width = 30
	typeInput.SetValue(taskType)

	priInput := textinput.New()
	priInput.Placeholder = "0-4 (0=critical, 2=medium, 4=backlog)"
	priInput.CharLimit = 1
	priInput.Width = 10
	priInput.SetValue(priority)

	return taskFormModel{
		fields:  []textinput.Model{titleInput, descInput, typeInput, priInput},
		labels:  []string{"Title", "Description", "Type", "Priority"},
		editing: editID,
	}
}

func (f *taskFormModel) nextField() {
	f.fields[f.activeField].Blur()
	f.activeField = (f.activeField + 1) % len(f.fields)
}

func (f *taskFormModel) prevField() {
	f.fields[f.activeField].Blur()
	f.activeField = (f.activeField - 1 + len(f.fields)) % len(f.fields)
}

func (f *taskFormModel) focusCmd() tea.Cmd {
	return f.fields[f.activeField].Focus()
}

func (f *taskFormModel) isEdit() bool {
	return f.editing != ""
}

func (m Model) renderTaskForm(width int) string {
	action := "New Task"
	if m.createForm.isEdit() {
		action = "Edit " + m.createForm.editing
	}
	header := headerStyle.Render(action)

	var rows []string
	for i, field := range m.createForm.fields {
		label := m.createForm.labels[i]
		prefix := "  "
		if i == m.createForm.activeField {
			prefix = selectedStyle.Render("> ")
		}
		rows = append(rows, prefix+labelStyle.Render(label+":")+field.View())
	}

	hint := dimStyle.Render("  Tab: next field  Shift+Tab: prev  Enter: submit  Esc: cancel")
	content := header + "\n" + strings.Join(rows, "\n") + "\n" + hint
	return panelStyle.Width(width).Render(content)
}
