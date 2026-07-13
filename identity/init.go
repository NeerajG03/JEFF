package identity

import (
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// GenerateOpts configures a new identity.
type GenerateOpts struct {
	// ID, when non-empty, is used as-is instead of a fresh UUID. Set this when
	// adopting an existing orchestrator so workers already bound to it keep
	// their orchestrator_id.
	ID string
	// Name is the human-readable label. When empty, it defaults to the basename
	// of Dir.
	Name string
	// Dir is the project directory; its basename is the fallback Name.
	Dir string
	// TmuxPane is the optional tmux pane binding (empty outside tmux).
	TmuxPane string
	// Now stamps CreatedAt; when nil, time.Now().UTC() is used. Injected for tests.
	Now func() time.Time
}

// Generate builds a new Identity from opts. It does not touch the filesystem.
func Generate(opts GenerateOpts) *Identity {
	id := opts.ID
	if id == "" {
		id = uuid.New().String()
	}
	name := opts.Name
	if name == "" {
		name = filepath.Base(opts.Dir)
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now()
	}
	return &Identity{
		ID:        id,
		Name:      name,
		CreatedAt: now.Format(time.RFC3339),
		TmuxPane:  opts.TmuxPane,
	}
}
