package tui

import (
	"fmt"
	"time"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/gig"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshInterval = 2 * time.Second

// Model is the root Bubbletea model for the crew dashboard.
type Model struct {
	crewStore  *crew.Store
	gigStore   *gig.Store
	sessions   []*crew.Session
	events     []*gig.Event
	selected   int
	paneOutput string
	input      inputModel
	lastEvent  time.Time
	width      int
	height     int
	err        error
}

type tickMsg time.Time

type refreshedMsg struct {
	sessions []*crew.Session
	events   []*gig.Event
	pane     string
}

type sentMsg struct{ err error }

// New creates a new dashboard model.
func New(crewStore *crew.Store, gigStore *gig.Store) Model {
	return Model{
		crewStore: crewStore,
		gigStore:  gigStore,
		input:     newInputModel(),
		lastEvent: time.Now().Add(-5 * time.Minute),
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
		m.events = msg.events
		m.paneOutput = msg.pane
		if len(m.events) > 0 {
			m.lastEvent = m.events[len(m.events)-1].Timestamp
		}
		// Clamp selection.
		if m.selected >= len(m.sessions) {
			m.selected = max(0, len(m.sessions)-1)
		}
		return m, nil

	case sentMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.input.close()
		return m, nil

	case tea.KeyMsg:
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
			return m, func() tea.Msg {
				_ = crew.Stop(m.crewStore, sess.TaskID)
				return tickMsg(time.Now())
			}
		}
		return m, nil

	case "r":
		return m, m.refreshCmd()

	case "a":
		if len(m.sessions) > 0 {
			sess := m.sessions[m.selected]
			target := sess.TmuxSession + ":" + sess.WindowName
			_ = crew.SelectWindow(target)
			return m, tea.Quit
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
			return sentMsg{err: err}
		}
	}

	// Forward to text input.
	var cmd tea.Cmd
	m.input.input, cmd = m.input.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	w := m.width - 2

	title := titleStyle.Render("JEFF Dashboard")

	// Session list.
	sessionPanel := renderSessionList(m.sessions, m.selected, m.gigStore, w)

	// Detail panel for selected session.
	var detailPanel string
	if len(m.sessions) > 0 && m.selected < len(m.sessions) {
		sess := m.sessions[m.selected]
		detailPanel = renderDetail(sess, m.gigStore, m.crewStore, m.paneOutput, w)
	}

	// Event feed.
	eventPanel := renderEvents(m.events, w)

	// Input overlay.
	inputPanel := renderInput(m.input, w)

	// Error display.
	errMsg := ""
	if m.err != nil {
		errMsg = lipgloss.NewStyle().Foreground(danger).Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Help bar.
	help := helpStyle.Render("j/k navigate  s send  a attach  c capture  x stop  r refresh  q quit")

	// Compose.
	parts := []string{title, sessionPanel}
	if detailPanel != "" {
		parts = append(parts, detailPanel)
	}
	parts = append(parts, eventPanel)
	if inputPanel != "" {
		parts = append(parts, inputPanel)
	}
	if errMsg != "" {
		parts = append(parts, errMsg)
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
		// Refresh tmux state.
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

		// Capture pane for selected session.
		pane := ""
		if len(sessions) > 0 && m.selected < len(sessions) {
			sess := sessions[m.selected]
			if crew.HasWindow(sess.WindowName) {
				target := sess.TmuxSession + ":" + sess.WindowName
				pane, _ = crew.CapturePane(target, 8)
			}
		}

		return refreshedMsg{
			sessions: sessions,
			events:   events,
			pane:     pane,
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
		pane, _ := crew.CapturePane(target, 20)
		return refreshedMsg{
			sessions: m.sessions,
			events:   m.events,
			pane:     pane,
		}
	}
}
