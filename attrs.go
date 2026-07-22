package jeff

import "github.com/NeerajG03/gig"

// JEFF custom attribute keys.
const (
	AttrRepos         = "repos"          // JSON array of repo names for a task
	AttrWorktreeSetup = "worktree_setup" // post-setup script path per repo

	// Phase 1 attributes (docs/roadmap.md "New Gig Attributes").
	AttrPersona        = "persona"         // string: persona used at pickup
	AttrSkillsLoaded   = "skills_loaded"   // object: JSON array of injected skill names
	AttrMemoryLoaded   = "memory_loaded"   // object: JSON array of memory scopes loaded
	AttrTeamSize       = "team_size"       // string: "1" for solo
	AttrOutcome        = "outcome"         // string: close reason ("done", "abandoned", ...)
	AttrRejectionCount = "rejection_count" // string: times a PR was sent back (not yet written)
)

// EnsureAttrs defines the custom attributes JEFF needs in gig.
// Idempotent — skips attributes that already exist.
func EnsureAttrs(store *gig.Store) error {
	attrs := []struct {
		Key  string
		Type gig.AttrType
		Desc string
	}{
		{AttrRepos, gig.AttrObject, "JSON array of repo names this task touches"},
		{AttrWorktreeSetup, gig.AttrString, "Post-setup script path run after worktree creation"},
		{AttrPersona, gig.AttrString, "Persona used to work this task"},
		{AttrSkillsLoaded, gig.AttrObject, "JSON array of skill names injected at pickup"},
		{AttrMemoryLoaded, gig.AttrObject, "JSON array of memory scopes loaded at pickup"},
		{AttrTeamSize, gig.AttrString, "Number of agents on this task (1 = solo)"},
		{AttrOutcome, gig.AttrString, "Task outcome recorded at close"},
		{AttrRejectionCount, gig.AttrString, "How many times the task's PR was rejected"},
	}

	for _, a := range attrs {
		// Check if already defined.
		if _, err := store.GetAttrDef(a.Key); err == nil {
			continue
		}
		if err := store.DefineAttr(a.Key, a.Type, a.Desc); err != nil {
			return err
		}
	}
	return nil
}
