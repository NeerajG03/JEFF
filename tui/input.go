package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

var msgTypes = []string{"nudge", "status", "divert", "normal"}

type inputModel struct {
	input     textinput.Model
	typeIdx   int
	active    bool
	targetID  string
}

func newInputModel() inputModel {
	ti := textinput.New()
	ti.Placeholder = "Type message..."
	ti.CharLimit = 500
	return inputModel{input: ti}
}

func (m *inputModel) open(targetID string) {
	m.active = true
	m.targetID = targetID
	m.input.SetValue("")
	m.input.Focus()
	m.typeIdx = 0
}

func (m *inputModel) close() {
	m.active = false
	m.input.Blur()
}

func (m *inputModel) cycleType() {
	m.typeIdx = (m.typeIdx + 1) % len(msgTypes)
}

func (m *inputModel) msgType() string {
	return msgTypes[m.typeIdx]
}

func (m *inputModel) value() string {
	return m.input.Value()
}

func renderInput(im inputModel, width int) string {
	if !im.active {
		return ""
	}

	typeLabel := lipgloss.NewStyle().
		Bold(true).
		Foreground(highlight).
		Render("[" + im.msgType() + "]")

	header := fmt.Sprintf("Send to %s  %s  (Tab: cycle type, Enter: send, Esc: cancel)",
		im.targetID, typeLabel)

	content := headerStyle.Render(header) + "\n" + im.input.View()
	return panelStyle.Width(width).Render(content)
}
