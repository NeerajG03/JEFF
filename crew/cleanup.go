package crew

import (
	"fmt"
)

// CleanupResult holds the results of a cleanup operation.
type CleanupResult struct {
	OrphanedWindows []TmuxWindow // tmux windows with no DB session
	StaleSessions   []string     // DB session task IDs with no tmux window
	StaleOrch       []string     // orchestrator IDs with no tmux session
}

// IsClean returns true if no cleanup was needed.
func (r *CleanupResult) IsClean() bool {
	return len(r.OrphanedWindows) == 0 &&
		len(r.StaleSessions) == 0 &&
		len(r.StaleOrch) == 0
}

// knownWindows are tmux windows that are part of jeff infrastructure, not workers.
var knownWindows = map[string]bool{
	DashboardWindowName: true,
	"orchestrator":      true,
}

// Cleanup reconciles tmux windows with the crew DB.
//
//  1. Lists all tmux windows in jeff and jeff-N sessions.
//  2. Compares against DB sessions — kills orphaned windows (no DB match).
//  3. Removes stale DB sessions (window gone).
//  4. Removes stale orchestrators (session gone).
//
// If dryRun is true, no destructive actions are taken — only the result is populated.
func Cleanup(store *Store, jeffHome string, dryRun bool) (*CleanupResult, error) {
	result := &CleanupResult{}

	// --- Step 1-2: Reconcile tmux windows vs DB sessions ---

	tmuxWindows, _ := ListAllJeffWindows()

	// Build a set of DB sessions keyed by "session:window".
	allSessions, err := store.ListSessions(false)
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
			result.OrphanedWindows = append(result.OrphanedWindows, tw)
		}
	}

	// Find stale DB sessions: exist in DB but not in tmux.
	for _, sess := range allSessions {
		if sess.Status == "done" || sess.Status == "failed" || sess.Status == "stopped" {
			continue // Already terminal, not stale — just old records.
		}
		key := sess.TmuxSession + ":" + SanitizeWindowName(sess.WindowName)
		if !tmuxWindowSet[key] {
			result.StaleSessions = append(result.StaleSessions, sess.TaskID)
		}
	}

	// Find stale orchestrators: DB says running but tmux session is gone.
	tmuxSessions, _ := ListAllJeffSessions()
	tmuxSessionSet := make(map[string]bool)
	for _, s := range tmuxSessions {
		tmuxSessionSet[s] = true
	}
	for _, o := range allOrch {
		if o.Status != "running" {
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

	// Kill orphaned tmux windows.
	for _, tw := range result.OrphanedWindows {
		target := tw.Session + ":" + tw.Window
		_ = KillWindow(target)
	}

	// Mark stale DB sessions as failed.
	for _, taskID := range result.StaleSessions {
		_ = store.UpdateStatus(taskID, "failed")
	}

	// Mark stale orchestrators as stopped.
	for _, orchID := range result.StaleOrch {
		_, _ = store.db.Exec(`UPDATE orchestrators SET status = 'stopped' WHERE id = ?`, orchID)
	}

	// Clean up empty jeff-N sessions after removing orphan windows.
	for _, sessName := range tmuxSessions {
		if sessName == TmuxSessionName {
			continue // Don't kill the main jeff session.
		}
		windows, err := ListSessionWindows(sessName)
		if err != nil || len(windows) == 0 {
			_ = KillSession(sessName)
		}
	}

	return result, nil
}

