package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/gig"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshInterval = 2 * time.Second

// Model is the root Bubbletea model for the crew dashboard.
type Model struct {
	crewStore *crew.Store
	gigStore  *gig.Store

	sessions   []*crew.Session
	events     []*gig.Event // accumulated events
	taskTitles map[string]string
	selected   int
	paneOutput string

	input     inputModel
	lastEvent time.Time
	width     int
	height    int
	err       error
	status    string // transient status message

	// Set before quitting to trigger tmux attach.
	AttachTarget string
}

type tickMsg time.Time

type refreshedMsg struct {
	sessions []*crew.Session
	events   []*gig.Event
	titles   map[string]string
	pane     string
}

type sentMsg struct {
	err    error
	taskID string
	typ    string
}

// New creates a new dashboard model.
func New(crewStore *crew.Store, gigStore *gig.Store) Model {
	return Model{
		crewStore:  crewStore,
		gigStore:   gigStore,
		input:      newInputModel(),
		lastEvent:  time.Now().Add(-10 * time.Minute),
		taskTitles: make(map[string]string),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.refreshCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(tickCmd(), m.refreshCmd())

	case refreshedMsg:
		m.sessions = msg.sessions
		// Accumulate events (append new, cap at 50).
		if len(msg.events) > 0 {
			m.events = append(m.events, msg.events...)
			if len(m.events) > 50 {
				m.events = m.events[len(m.events)-50:]
			}
			m.lastEvent = m.events[len(m.events)-1].Timestamp
		}
		for k, v := range msg.titles {
			m.taskTitles[k] = v
		}
		m.paneOutput = msg.pane
		if m.selected >= len(m.sessions) {
			m.selected = max(0, len(m.sessions)-1)
		}
		return m, nil

	case sentMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Sent %s to %s", msg.typ, msg.taskID)
		}
		m.input.close()
		return m, nil

	case tea.KeyMsg:
		// Clear transient status on any keypress.
		m.err = nil
		m.status = ""
		if m.input.active {
			return m.updateInput(msg)
		}
		return m.updateNormal(msg)
	}

	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.selected < len(m.sessions)-1 {
			m.selected++
			m.paneOutput = ""
		}
		return m, nil

	case "k", "up":
		if m.selected > 0 {
			m.selected--
			m.paneOutput = ""
		}
		return m, nil

	case "s":
		if len(m.sessions) > 0 {
			m.input.open(m.sessions[m.selected].TaskID)
			return m, m.input.input.Focus()
		}
		return m, nil

	case "c":
		if len(m.sessions) > 0 {
			return m, m.captureCmd()
		}
		return m, nil

	case "x":
		if len(m.sessions) > 0 {
			sess := m.sessions[m.selected]
			if sess.Status == "running" || sess.Status == "starting" {
				taskID := sess.TaskID
				return m, func() tea.Msg {
					_ = crew.Stop(m.crewStore, taskID)
					return tickMsg(time.Now())
				}
			}
		}
		return m, nil

	case "r":
		return m, m.refreshCmd()

	case "enter", "a":
		if len(m.sessions) > 0 {
			sess := m.sessions[m.selected]
			if crew.HasWindow(sess.WindowName) {
				m.AttachTarget = sess.WindowName
				return m, tea.Quit
			}
			m.status = fmt.Sprintf("No tmux window for %s", sess.TaskID)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.input.close()
		return m, nil

	case "tab":
		m.input.cycleType()
		return m, nil

	case "enter":
		content := m.input.value()
		if content == "" {
			m.input.close()
			return m, nil
		}
		taskID := m.input.targetID
		msgType := crew.MessageType(m.input.msgType())
		return m, func() tea.Msg {
			_, err := crew.Send(m.crewStore, taskID, msgType, content)
			return sentMsg{err: err, taskID: taskID, typ: string(msgType)}
		}
	}

	var cmd tea.Cmd
	m.input.input, cmd = m.input.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	w := m.width - 2

	// --- Title bar ---
	running := 0
	for _, s := range m.sessions {
		if s.Status == "running" || s.Status == "starting" {
			running++
		}
	}
	title := titleStyle.Render(" JEFF Dashboard ") +
		dimStyle.Render(fmt.Sprintf("  %d workers", running))

	// --- Session list (natural height) ---
	sessionPanel := renderSessionList(m.sessions, m.selected, m.gigStore, m.taskTitles, w)

	// --- Detail panel (natural height) ---
	var detailPanel string
	if len(m.sessions) > 0 && m.selected < len(m.sessions) {
		sess := m.sessions[m.selected]
		taskTitle := m.taskTitles[sess.TaskID]
		detailPanel = renderDetail(sess, taskTitle, m.gigStore, m.crewStore, m.paneOutput, w)
	} else if len(m.sessions) == 0 {
		detailPanel = panelStyle.Width(w).Render(
			dimStyle.Render("Start a worker: jeff crew start <gig-id> --persona jenko"))
	}

	// --- Activity feed (natural height) ---
	eventPanel := renderEvents(m.events, w)

	// --- Input overlay ---
	inputPanel := renderInput(m.input, w)

	// --- Status / help bar ---
	helpText := "j/k navigate  enter attach  s send  c capture  x stop  r refresh  q quit"
	if m.input.active {
		helpText = "Tab cycle type  Enter send  Esc cancel"
	}
	help := helpStyle.Render(helpText)

	if m.status != "" {
		help = lipgloss.NewStyle().Foreground(warning).Render(m.status) + "  " + help
	}

	// --- Compose ---
	parts := []string{title, sessionPanel, detailPanel, eventPanel}
	if inputPanel != "" {
		parts = append(parts, inputPanel)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		var isTaskClosed func(string) bool
		if m.gigStore != nil {
			isTaskClosed = func(taskID string) bool {
				t, err := m.gigStore.Get(taskID)
				return err == nil && t.Status.IsTerminal()
			}
		}
		_ = crew.Refresh(m.crewStore, isTaskClosed)

		sessions, _ := m.crewStore.ListSessions(false)

		var events []*gig.Event
		if m.gigStore != nil {
			events, _ = m.gigStore.EventsSince(m.lastEvent)
		}

		// Fetch task titles.
		titles := make(map[string]string)
		if m.gigStore != nil {
			for _, sess := range sessions {
				if t, err := m.gigStore.Get(sess.TaskID); err == nil {
					titles[sess.TaskID] = t.Title
				}
			}
		}

		// Pane capture is NOT automatic — only on explicit 'c' keypress.
		return refreshedMsg{
			sessions: sessions,
			events:   events,
			titles:   titles,
			pane:     "", // empty = don't show pane
		}
	}
}

func (m Model) captureCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected >= len(m.sessions) {
			return nil
		}
		sess := m.sessions[m.selected]
		if !crew.HasWindow(sess.WindowName) {
			return nil
		}
		target := sess.TmuxSession + ":" + sess.WindowName
		pane, _ := crew.CapturePane(target, 30)

		titles := make(map[string]string)
		for k, v := range m.taskTitles {
			titles[k] = v
		}
		return refreshedMsg{
			sessions: m.sessions,
			events:   nil, // no new events
			titles:   titles,
			pane:     pane,
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	return s + strings.Repeat(" ", w-len(s))
}
