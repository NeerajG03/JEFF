package jeff

import "github.com/neerajg/gig"

// JEFF custom attribute keys.
const (
	AttrRepos          = "repos"           // JSON array of repo names for a task
	AttrWorktreeSetup  = "worktree_setup"  // post-setup script path per repo
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
