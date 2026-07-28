package crew

import (
	"fmt"
)

// CleanupResult holds the results of a cleanup operation.
type CleanupResult struct {
	OrphanedWindows []TmuxWindow // dead-pane tmux windows with no DB session — killed
	LiveOrphans     []TmuxWindow // tmux windows with no DB session but a LIVE pane — never killed, reported only
	StaleSessions   []string     // DB session task IDs with no tmux window
	StaleOrch       []string     // orchestrator IDs with no tmux session
	// SkippedNoState is set when the crew DB has no session and no orchestrator
	// rows at all while jeff-managed worker windows exist in tmux. That almost
	// always means this process resolved a different JEFF_HOME (different
	// jeff.db) than the one that started the workers — killing windows based on
	// a DB that knows nothing would massacre live workers (the gig class where
	// every worker window closed at once), so the orphan sweep is skipped.
	SkippedNoState bool
}

// IsClean returns true if no cleanup was needed and nothing abnormal was found.
func (r *CleanupResult) IsClean() bool {
	return len(r.OrphanedWindows) == 0 &&
		len(r.LiveOrphans) == 0 &&
		len(r.StaleSessions) == 0 &&
		len(r.StaleOrch) == 0 &&
		!r.SkippedNoState
}

// knownWindows are tmux windows that are part of jeff infrastructure, not workers.
var knownWindows = map[string]bool{
	DashboardWindowName: true,
	"orchestrator":      true,
}

// Cleanup reconciles tmux windows with the crew DB.
//
//  1. Lists all tmux windows in jeff and jeff-N sessions (with pane liveness).
//  2. Compares against DB sessions — kills orphaned windows (no DB match)
//     ONLY when their pane is dead (a remain-on-exit leftover). A window whose
//     pane is alive is NEVER killed: a live pane means a shell or agent may
//     still be working, and any DB↔tmux view mismatch (renamed window, a
//     different JEFF_HOME resolving to a different jeff.db, a worker started
//     a moment ago whose row isn't visible yet) must not take workers down.
//  3. Removes stale DB sessions (window gone). Sessions whose tmux windows
//     could not be listed this pass are left untouched — a transient tmux
//     failure must not mass-fail live workers (same rule as Refresh).
//  4. Removes stale orchestrators (session gone).
//
// Killing a session's last window destroys the session in tmux, so no
// separate empty-session sweep is needed (the old sweep also killed sessions
// whose windows merely failed to LIST — a transient error nuked live crews).
//
// If dryRun is true, no destructive actions are taken — only the result is populated.
func Cleanup(store *Store, jeffHome string, dryRun bool) (*CleanupResult, error) {
	result := &CleanupResult{}

	// --- Step 1-2: Reconcile tmux windows vs DB sessions ---

	sessions, _ := ListAllJeffSessions()
	var tmuxWindows []WindowInfo
	listFailed := make(map[string]bool)
	for _, sess := range sessions {
		infos, err := ListSessionWindowInfo(sess)
		if err != nil {
			// Can't see this session's windows this pass. Do NOT treat it as
			// empty: skip its workers for both orphan and stale detection.
			listFailed[sess] = true
			continue
		}
		tmuxWindows = append(tmuxWindows, infos...)
	}

	// Build a set of DB sessions keyed by "session:window".
	allSessions, err := store.ListSessions(false, "")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	dbWindowSet := make(map[string]*Session)
	for _, sess := range allSessions {
		key := sess.TmuxSession + ":" + SanitizeWindowName(sess.WindowName)
		dbWindowSet[key] = sess
	}

	// Build a set of orchestrators keyed by tmux session name.
	allOrch, err := store.ListOrchestrators(false)
	if err != nil {
		return nil, fmt.Errorf("list orchestrators: %w", err)
	}
	orchSet := make(map[string]*Orchestrator)
	for _, o := range allOrch {
		orchSet[o.TmuxSession] = o
	}

	// Find orphaned tmux windows: exist in tmux but not in DB.
	tmuxWindowSet := make(map[string]bool)
	var deadOrphans []WindowInfo
	for _, tw := range tmuxWindows {
		key := tw.Session + ":" + tw.Window
		tmuxWindowSet[key] = true

		if knownWindows[tw.Window] {
			continue
		}
		// Check if this is an orchestrator window (session is jeff-N and window is "orchestrator").
		if _, isOrch := orchSet[tw.Session]; isOrch && tw.Window == "orchestrator" {
			continue
		}
		if _, found := dbWindowSet[key]; !found {
			if tw.PaneDead {
				deadOrphans = append(deadOrphans, tw)
				result.OrphanedWindows = append(result.OrphanedWindows, TmuxWindow{Session: tw.Session, Window: tw.Window})
			} else {
				result.LiveOrphans = append(result.LiveOrphans, TmuxWindow{Session: tw.Session, Window: tw.Window})
			}
		}
	}

	// A DB with no crew state at all cannot arbitrate which windows are
	// garbage. If worker windows exist anyway, this is almost certainly a
	// different jeff.db than the one that started them (JEFF_HOME divergence)
	// — refuse the orphan sweep instead of killing everything.
	if len(allSessions) == 0 && len(allOrch) == 0 &&
		(len(result.OrphanedWindows) > 0 || len(result.LiveOrphans) > 0) {
		result.SkippedNoState = true
		result.LiveOrphans = append(result.LiveOrphans, result.OrphanedWindows...)
		result.OrphanedWindows = nil
		deadOrphans = nil
	}

	// Find stale DB sessions: exist in DB but not in tmux.
	for _, sess := range allSessions {
		if sess.Status == "done" || sess.Status == "failed" || sess.Status == "stopped" {
			continue // Already terminal, not stale — just old records.
		}
		if listFailed[sess.TmuxSession] {
			continue // Couldn't enumerate that session this pass — don't false-fail.
		}
		key := sess.TmuxSession + ":" + SanitizeWindowName(sess.WindowName)
		if !tmuxWindowSet[key] {
			result.StaleSessions = append(result.StaleSessions, sess.TaskID)
		}
	}

	// Find stale orchestrators: DB says running but tmux session is gone.
	tmuxSessionSet := make(map[string]bool)
	for _, s := range sessions {
		tmuxSessionSet[s] = true
	}
	for _, o := range allOrch {
		if o.Status != "running" {
			continue
		}
		// Durable identities registered outside tmux have no session; their
		// liveness isn't tmux-bound, so they are never "stale" on this basis.
		if o.TmuxSession == "" {
			continue
		}
		if !tmuxSessionSet[o.TmuxSession] {
			result.StaleOrch = append(result.StaleOrch, o.ID)
		}
	}

	if dryRun {
		return result, nil
	}

	// --- Execute cleanup ---

	// Kill dead orphaned tmux windows by window ID (immune to renamed names
	// and to tmux target parsing of dots). Killing a session's last window
	// destroys the session automatically.
	for _, tw := range deadOrphans {
		_ = KillWindow(tw.WindowID)
	}

	// Mark stale DB sessions as failed.
	for _, taskID := range result.StaleSessions {
		_ = store.UpdateStatus(taskID, "failed")
	}

	// Mark stale orchestrators as stopped.
	for _, orchID := range result.StaleOrch {
		_, _ = store.db.Exec(`UPDATE orchestrators SET status = 'stopped' WHERE id = ?`, orchID)
	}

	return result, nil
}
