package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
)

var msgTypes = []string{"nudge", "status", "divert", "normal"}

type inputModel struct {
	input    textinput.Model
	typeIdx  int
	active   bool
	targetID string
	crewMode bool // when true, input is for persona name (not message)
}

func newInputModel() inputModel {
	ti := textinput.New()
	ti.Placeholder = "Type message..."
	ti.CharLimit = 500
	return inputModel{input: ti}
}

func (m *inputModel) open(targetID string) {
	m.active = true
	m.crewMode = false
	m.targetID = targetID
	m.input.SetValue("")
	m.input.Placeholder = "Type message..."
	m.input.Focus()
	m.typeIdx = 0
}

func (m *inputModel) openForCrew(taskID string) {
	m.active = true
	m.crewMode = true
	m.targetID = taskID
	m.input.SetValue("jenko")
	m.input.Placeholder = "Persona name (jenko, schmidt, hardy, eric)"
	m.input.Focus()
}

func (m *inputModel) close() {
	m.active = false
	m.crewMode = false
	m.input.Blur()
}

func (m *inputModel) cycleType() {
	if m.crewMode {
		return
	}
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

	if im.crewMode {
		header := fmt.Sprintf("Start worker for %s — enter persona name:", im.targetID)
		content := headerStyle.Render(header) + "\n" + im.input.View()
		return panelStyle.Width(width).Render(content)
	}

	typeLabel := lipglossBlue.Bold(true).Render("[" + im.msgType() + "]")
	header := fmt.Sprintf("Send to %s  %s  (Tab: cycle type, Enter: send, Esc: cancel)",
		im.targetID, typeLabel)

	content := headerStyle.Render(header) + "\n" + im.input.View()
	return panelStyle.Width(width).Render(content)
}
