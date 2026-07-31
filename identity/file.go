// Package identity manages a durable, per-project orchestrator identity that is
// decoupled from tmux. Before this package, orchestrator identity was derived
// from the tmux session name / pane, so any orchestrator running OUTSIDE tmux
// (Cursor, VS Code, a plain terminal, CI) silently fell through to a shared
// default with an empty orchestrator_id — stranding workers and breaking
// stop-notifications, crew scoping, worker→orch asks, and stall detection.
//
// The identity now lives in an on-disk JSON file (.jeff/orchestrator.json) with
// a stable id that survives shell restarts and process relaunches, and works
// regardless of whether the orchestrator is inside tmux. Tmux binding remains as
// an optional enhancement, recorded in the file's tmux_pane field.
package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// ProjectDirName is the per-project identity directory (holds FileName). It
	// is a per-directory marker in the manner of .git — deliberately NOT the JEFF
	// home. The two used to share the name `DirName`, and the collision is what
	// sent `orchestrator init --global` into a stray ~/.jeff beside a real home
	// (#85): "<home>/.jeff/..." reads as "inside the home" but meant "the marker
	// dir in $HOME".
	ProjectDirName = ".jeff"
	// FileName is the per-project identity file within ProjectDirName.
	FileName = "orchestrator.json"
	// GlobalFileName is the machine-wide default identity file. It lives directly
	// in the resolved JEFF home — see GlobalFilePathIn.
	GlobalFileName = "default-orchestrator.json"
)

// Identity is the durable orchestrator identity persisted to disk.
type Identity struct {
	// ID is the stable identifier used as orchestrator_id in the DB. It is a
	// UUID for fresh identities, or the adopted tmux orchestrator's id.
	ID string `json:"id"`
	// Name is a human-readable label (from --name, the tmux session name, or
	// the basename of the project directory).
	Name string `json:"name"`
	// CreatedAt is an ISO-8601 (RFC3339) timestamp.
	CreatedAt string `json:"created_at"`
	// TmuxPane is set ONLY when the identity was created inside a tmux pane. It
	// is an optional enhancement enabling direct pane notification routing; when
	// empty, workers fall back to DB/poll-based signalling.
	TmuxPane string `json:"tmux_pane,omitempty"`
}

// ProjectFilePath returns <dir>/.jeff/orchestrator.json — the per-directory
// marker for a project root.
func ProjectFilePath(dir string) string {
	return filepath.Join(dir, ProjectDirName, FileName)
}

// GlobalFilePathIn returns <jeffHome>/default-orchestrator.json — the machine-wide
// default identity, inside the resolved JEFF home.
//
// The parameter is the JEFF home, NOT $HOME. For a default install the two produce
// the same path (~/.jeff/default-orchestrator.json), so existing installs are
// unaffected; for a relocated home the file now follows the home instead of being
// stranded in a stray ~/.jeff (#85). The name says "In" precisely so a caller
// cannot pass $HOME by accident and get a silently wrong path.
func GlobalFilePathIn(jeffHome string) string {
	return filepath.Join(jeffHome, GlobalFileName)
}

// Read loads and validates an identity file. A parse error or a missing id is
// returned as an error (never silently ignored) so callers fail loud on a
// corrupt file rather than falling through to a shared default.
func Read(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("malformed identity file %s: %w", path, err)
	}
	if id.ID == "" {
		return nil, fmt.Errorf("malformed identity file %s: missing \"id\"", path)
	}
	return &id, nil
}

// readIfExists returns (nil, nil) when the file does not exist, (identity, nil)
// on a valid file, and (nil, err) on a genuine read/parse error. A malformed
// file that DOES exist is a hard error — never a silent "not found".
func readIfExists(path string) (*Identity, error) {
	id, err := Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return id, nil
}

// Write atomically persists the identity to path. It writes to a sibling .tmp
// file and renames into place, so an interrupted write never leaves a corrupt
// file behind. Parent directories are created as needed.
func Write(path string, id *Identity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create identity dir: %w", err)
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp identity file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename identity file into place: %w", err)
	}
	return nil
}
