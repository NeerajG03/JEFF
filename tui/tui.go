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

// Tab identifies which tab is active.
type Tab int

const (
	TabCrew Tab = iota
	TabGigs
)

// Model is the root Bubbletea model for the dashboard.
type Model struct {
	crewStore *crew.Store
	gigStore  *gig.Store

	// Shared state.
	tab    Tab
	width  int
	height int
	status string // transient status bar message
	err    error

	// Crew tab state.
	sessions   []*crew.Session
	events     []*gig.Event
	taskTitles map[string]string
	crewSel    int
	paneOutput string
	input      inputModel
	lastEvent  time.Time

	// Gigs tab state.
	readyTasks []*gig.Task
	allTasks   []*gig.Task // in_progress tasks
	gigsSel    int
	creating   bool // create-task form active
	createForm createFormModel

	// Set before quitting to trigger tmux attach.
	AttachTarget string
}

type tickMsg time.Time

type refreshedMsg struct {
	sessions   []*crew.Session
	events     []*gig.Event
	titles     map[string]string
	pane       string
	readyTasks []*gig.Task
	allTasks   []*gig.Task
}

type sentMsg struct {
	err    error
	taskID string
	typ    string
}

type taskCreatedMsg struct {
	err  error
	task *gig.Task
}

type crewStartedMsg struct {
	err    error
	taskID string
}

// New creates a new dashboard model.
func New(crewStore *crew.Store, gigStore *gig.Store) Model {
	return Model{
		crewStore:  crewStore,
		gigStore:   gigStore,
		input:      newInputModel(),
		createForm: newCreateForm(),
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
		m.readyTasks = msg.readyTasks
		m.allTasks = msg.allTasks
		if m.crewSel >= len(m.sessions) {
			m.crewSel = max(0, len(m.sessions)-1)
		}
		if m.gigsSel >= len(m.gigsListItems()) {
			m.gigsSel = max(0, len(m.gigsListItems())-1)
		}
		return m, nil

	case sentMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Sent %s to %s", msg.typ, msg.taskID)
		}
		m.input.close()
		return m, nil

	case taskCreatedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error creating task: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Created %s: %s", msg.task.ID, msg.task.Title)
		}
		m.creating = false
		return m, m.refreshCmd()

	case crewStartedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error starting crew: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Started crew for %s", msg.taskID)
			m.tab = TabCrew // switch to crew tab to see the new worker
		}
		return m, m.refreshCmd()

	case tea.KeyMsg:
		m.err = nil
		m.status = ""

		// Form/input modes take priority.
		if m.input.active {
			return m.updateInput(msg)
		}
		if m.creating {
			return m.updateCreateForm(msg)
		}

		// Global keys (work on any tab).
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.tab = TabCrew
			return m, nil
		case "2":
			m.tab = TabGigs
			return m, nil
		case "r":
			return m, m.refreshCmd()
		}

		// Tab-specific keys.
		switch m.tab {
		case TabCrew:
			return m.updateCrew(msg)
		case TabGigs:
			return m.updateGigs(msg)
		}
	}

	return m, nil
}

// --- Crew tab ---

func (m Model) updateCrew(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.crewSel < len(m.sessions)-1 {
			m.crewSel++
			m.paneOutput = ""
		}
	case "k", "up":
		if m.crewSel > 0 {
			m.crewSel--
			m.paneOutput = ""
		}
	case "s":
		if len(m.sessions) > 0 {
			m.input.open(m.sessions[m.crewSel].TaskID)
			return m, m.input.input.Focus()
		}
	case "c":
		if len(m.sessions) > 0 {
			return m, m.captureCmd()
		}
	case "x":
		if len(m.sessions) > 0 {
			sess := m.sessions[m.crewSel]
			if sess.Status == "running" || sess.Status == "starting" {
				taskID := sess.TaskID
				return m, func() tea.Msg {
					_ = crew.Stop(m.crewStore, taskID)
					return tickMsg(time.Now())
				}
			}
		}
	case "enter", "a":
		if len(m.sessions) > 0 {
			sess := m.sessions[m.crewSel]
			if crew.HasWindow(sess.WindowName) {
				m.AttachTarget = sess.WindowName
				return m, tea.Quit
			}
			m.status = fmt.Sprintf("No tmux window for %s", sess.TaskID)
		}
	}
	return m, nil
}

// --- Gigs tab ---

func (m Model) updateGigs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.gigsListItems()
	switch msg.String() {
	case "j", "down":
		if m.gigsSel < len(items)-1 {
			m.gigsSel++
		}
	case "k", "up":
		if m.gigsSel > 0 {
			m.gigsSel--
		}
	case "n":
		m.creating = true
		m.createForm = newCreateForm()
		return m, m.createForm.fields[0].Focus()
	case "w":
		// Start a crew worker for the selected ready task.
		if m.gigsSel < len(items) {
			task := items[m.gigsSel]
			m.input.openForCrew(task.ID)
			return m, m.input.input.Focus()
		}
	}
	return m, nil
}

func (m Model) gigsListItems() []*gig.Task {
	// Combine: ready tasks first, then in_progress.
	var items []*gig.Task
	items = append(items, m.readyTasks...)
	items = append(items, m.allTasks...)
	return items
}

// --- Input handling (shared) ---

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

		if m.input.crewMode {
			// Starting a crew worker — content is the persona name.
			taskID := m.input.targetID
			persona := content
			m.input.close()
			return m, func() tea.Msg {
				_, err := crew.Send(m.crewStore, taskID, crew.MsgNormal, "")
				// Actually start the crew via CLI.
				_ = err
				return crewStartedMsg{taskID: taskID, err: m.startCrewWorker(taskID, persona)}
			}
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

func (m Model) startCrewWorker(taskID, persona string) error {
	// Claim task.
	if _, err := m.gigStore.Claim(taskID, "jeff"); err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	// We can't run the full pickupTask from here (it's in cmd/jeff).
	// Instead, use the crew store to record a session that the user
	// can then resume with `jeff crew start` from CLI.
	// For now, return an error guiding the user.
	return fmt.Errorf("run: jeff crew start %s --persona %s", taskID, persona)
}

// --- Create form handling ---

func (m Model) updateCreateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating = false
		return m, nil
	case "tab":
		m.createForm.nextField()
		return m, m.createForm.focusCmd()
	case "shift+tab":
		m.createForm.prevField()
		return m, m.createForm.focusCmd()
	case "enter":
		if m.createForm.activeField == len(m.createForm.fields)-1 {
			// Submit on enter from last field.
			return m, m.submitCreateForm()
		}
		m.createForm.nextField()
		return m, m.createForm.focusCmd()
	}

	var cmd tea.Cmd
	f := m.createForm.activeField
	m.createForm.fields[f], cmd = m.createForm.fields[f].Update(msg)
	return m, cmd
}

func (m Model) submitCreateForm() tea.Cmd {
	title := m.createForm.fields[0].Value()
	desc := m.createForm.fields[1].Value()
	if title == "" {
		m.status = "Title is required"
		return nil
	}
	return func() tea.Msg {
		params := gig.CreateParams{
			Title:       title,
			Description: desc,
		}
		task, err := m.gigStore.Create(params)
		return taskCreatedMsg{err: err, task: task}
	}
}

// --- View ---

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	w := m.width

	// Tab bar.
	tabBar := m.renderTabBar(w)

	// Tab content.
	var content string
	switch m.tab {
	case TabCrew:
		content = m.viewCrew(w)
	case TabGigs:
		content = m.viewGigs(w)
	}

	// Status / help bar.
	helpText := m.helpText()
	help := helpStyle.Render(helpText)
	if m.status != "" {
		help = lipgloss.NewStyle().Foreground(gruvYellow).Render(m.status) + "  " + help
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, help)
}

func (m Model) renderTabBar(w int) string {
	crewLabel := "  Crew  "
	gigsLabel := "  Gigs  "

	activeTab := tabActiveStyle
	inactiveTab := tabInactiveStyle

	var tabs []string
	if m.tab == TabCrew {
		tabs = append(tabs, activeTab.Render(crewLabel))
		tabs = append(tabs, inactiveTab.Render(gigsLabel))
	} else {
		tabs = append(tabs, inactiveTab.Render(crewLabel))
		tabs = append(tabs, activeTab.Render(gigsLabel))
	}

	running := 0
	for _, s := range m.sessions {
		if s.Status == "running" || s.Status == "starting" {
			running++
		}
	}

	tabRow := lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)
	badge := dimStyle.Render(fmt.Sprintf("  %d workers  %d ready", running, len(m.readyTasks)))

	titleRow := titleStyle.Render(" JEFF ") + "  " + tabRow + badge
	separator := lipgloss.NewStyle().Foreground(gruvBg2).Render(strings.Repeat("─", w))

	return titleRow + "\n" + separator
}

func (m Model) viewCrew(w int) string {
	pw := w - 2 // panel inner width

	sessionPanel := renderSessionList(m.sessions, m.crewSel, m.gigStore, m.taskTitles, pw)

	var detailPanel string
	if len(m.sessions) > 0 && m.crewSel < len(m.sessions) {
		sess := m.sessions[m.crewSel]
		title := m.taskTitles[sess.TaskID]
		detailPanel = renderDetail(sess, title, m.gigStore, m.crewStore, m.paneOutput, pw)
	}

	eventPanel := renderEvents(m.events, pw)

	inputPanel := renderInput(m.input, pw)

	parts := []string{sessionPanel}
	if detailPanel != "" {
		parts = append(parts, detailPanel)
	}
	parts = append(parts, eventPanel)
	if inputPanel != "" {
		parts = append(parts, inputPanel)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) viewGigs(w int) string {
	pw := w - 2

	// Task list: ready tasks + in_progress tasks.
	taskPanel := m.renderGigsList(pw)

	// Detail for selected task.
	detailPanel := m.renderGigDetail(pw)

	// Create form overlay.
	formPanel := ""
	if m.creating {
		formPanel = m.renderCreateForm(pw)
	}

	// Input overlay for crew start (persona picker).
	inputPanel := ""
	if m.input.active && m.input.crewMode {
		inputPanel = renderInput(m.input, pw)
	}

	parts := []string{taskPanel}
	if detailPanel != "" {
		parts = append(parts, detailPanel)
	}
	if formPanel != "" {
		parts = append(parts, formPanel)
	}
	if inputPanel != "" {
		parts = append(parts, inputPanel)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) helpText() string {
	if m.creating {
		return "Tab next field  Enter submit/next  Esc cancel"
	}
	if m.input.active {
		if m.input.crewMode {
			return "Type persona name (jenko/schmidt/hardy/eric)  Enter start  Esc cancel"
		}
		return "Tab cycle type  Enter send  Esc cancel"
	}
	base := "1 crew  2 gigs  r refresh  q quit"
	switch m.tab {
	case TabCrew:
		return base + "  |  j/k navigate  enter attach  s send  c capture  x stop"
	case TabGigs:
		return base + "  |  j/k navigate  n new task  w start worker"
	}
	return base
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

		titles := make(map[string]string)
		if m.gigStore != nil {
			for _, sess := range sessions {
				if t, err := m.gigStore.Get(sess.TaskID); err == nil {
					titles[sess.TaskID] = t.Title
				}
			}
		}

		// Fetch gig tasks.
		var readyTasks []*gig.Task
		var allTasks []*gig.Task
		if m.gigStore != nil {
			readyTasks, _ = m.gigStore.Ready("")
			allTasks, _ = m.gigStore.List(gig.ListParams{
				Status: ptrStatus(gig.StatusInProgress),
			})
		}

		return refreshedMsg{
			sessions:   sessions,
			events:     events,
			titles:     titles,
			pane:       "",
			readyTasks: readyTasks,
			allTasks:   allTasks,
		}
	}
}

func (m Model) captureCmd() tea.Cmd {
	return func() tea.Msg {
		if m.crewSel >= len(m.sessions) {
			return nil
		}
		sess := m.sessions[m.crewSel]
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
			titles:   titles,
			pane:     pane,
		}
	}
}

func ptrStatus(s gig.Status) *gig.Status { return &s }

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
	if n <= 3 {
		return s
	}
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
