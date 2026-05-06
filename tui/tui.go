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
	formActive bool // task form (create or edit) active
	createForm taskFormModel

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
		m.formActive = false
		return m, m.refreshCmd()

	case tea.KeyMsg:
		m.err = nil
		m.status = ""

		// Form/input modes take priority.
		if m.input.active {
			return m.updateInput(msg)
		}
		if m.formActive {
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
		m.formActive = true
		m.createForm = newCreateForm()
		return m, m.createForm.fields[0].Focus()
	case "e":
		// Edit the selected task.
		if m.gigsSel < len(items) {
			task := items[m.gigsSel]
			m.formActive = true
			m.createForm = newEditForm(task)
			return m, m.createForm.fields[0].Focus()
		}
	case "w":
		// Start a crew worker — guide user to CLI (can't run full pickup from TUI).
		if m.gigsSel < len(items) {
			task := items[m.gigsSel]
			m.status = fmt.Sprintf("Run: jeff crew start %s --persona jenko", task.ID)
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


// --- Create form handling ---

func (m Model) updateCreateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.formActive = false
		return m, nil
	case "tab":
		m.createForm.nextField()
		return m, m.createForm.focusCmd()
	case "shift+tab":
		m.createForm.prevField()
		return m, m.createForm.focusCmd()
	case "enter":
		if m.createForm.activeField == len(m.createForm.fields)-1 {
			return m, m.submitTaskForm()
		}
		m.createForm.nextField()
		return m, m.createForm.focusCmd()
	}

	var cmd tea.Cmd
	f := m.createForm.activeField
	m.createForm.fields[f], cmd = m.createForm.fields[f].Update(msg)
	return m, cmd
}

func (m Model) submitTaskForm() tea.Cmd {
	title := m.createForm.fields[0].Value()
	desc := m.createForm.fields[1].Value()
	taskType := m.createForm.fields[2].Value()
	priStr := m.createForm.fields[3].Value()

	if title == "" {
		return nil
	}

	var pri gig.Priority = gig.P2
	if len(priStr) == 1 && priStr[0] >= '0' && priStr[0] <= '4' {
		pri = gig.Priority(priStr[0] - '0')
	}

	tt := gig.TypeTask
	switch taskType {
	case "bug":
		tt = gig.TypeBug
	case "feature":
		tt = gig.TypeFeature
	case "epic":
		tt = gig.TypeEpic
	case "chore":
		tt = gig.TypeChore
	}

	if m.createForm.isEdit() {
		editID := m.createForm.editing
		return func() tea.Msg {
			titleP := title
			descP := desc
			priP := pri
			_, err := m.gigStore.Update(editID, gig.UpdateParams{
				Title:       &titleP,
				Description: &descP,
				Priority:    &priP,
			}, "jeff-dashboard")
			if err != nil {
				return taskCreatedMsg{err: err}
			}
			task, _ := m.gigStore.Get(editID)
			return taskCreatedMsg{err: nil, task: task}
		}
	}

	return func() tea.Msg {
		task, err := m.gigStore.Create(gig.CreateParams{
			Title:       title,
			Description: desc,
			Type:        tt,
			Priority:    pri,
		})
		return taskCreatedMsg{err: err, task: task}
	}
}

// --- View ---

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	w := m.width - 1 // leave 1 char margin to avoid wrapping

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
	pw := w // panels stretch to full width

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
	pw := w

	// Task list: ready tasks + in_progress tasks.
	taskPanel := m.renderGigsList(pw)

	// Detail for selected task.
	detailPanel := m.renderGigDetail(pw)

	// Task form overlay (create or edit).
	formPanel := ""
	if m.formActive {
		formPanel = m.renderTaskForm(pw)
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
	if m.formActive {
		return "Tab next field  Shift+Tab prev  Enter submit/next  Esc cancel"
	}
	if m.input.active {
		return "Tab cycle type  Enter send  Esc cancel"
	}
	base := "1 crew  2 gigs  r refresh  q quit"
	switch m.tab {
	case TabCrew:
		return base + "  |  j/k navigate  enter attach  s send  c capture  x stop"
	case TabGigs:
		return base + "  |  j/k navigate  n new  e edit  w start worker"
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

		sessions, _ := m.crewStore.ListSessions(false, "")

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
		target := crew.SessionTarget(sess.TmuxSession, sess.WindowName)
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
