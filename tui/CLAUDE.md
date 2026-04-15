# tui/ — Bubbletea dashboard (`jeff dashboard`)

Terminal UI with two tabs: Crew (session monitoring + messaging) and Gigs (task management).

## File roles

| File | What it does |
|------|-------------|
| `tui.go` | Root Model, Init/Update/View, tab switching, tick |
| `sessions.go` | Crew tab — session list, event log, messaging |
| `gigs.go` | Gigs tab — ready/in-progress tasks, form |
| `events.go` | Event rendering helpers |
| `input.go` | Input field model (for crew messaging) |
| `style.go` | Lipgloss styles (gruvbox dark theme) |

## Key facts

- Refresh: 2s polling via `tea.Tick`
- Data sources: `crew.Store` (SQLite) + `gig.Store`
- Theme: gruvbox dark (hardcoded in `style.go`)
- Dependencies: `charmbracelet/bubbletea`, `charmbracelet/lipgloss`

## Bubbletea pattern

Model holds all state. `Update()` handles `tea.Msg` events + returns `Cmd`. `View()` is pure render from state. Never mutate state in `View()`.
