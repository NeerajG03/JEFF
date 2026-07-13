package identity

import (
	"os"
	"path/filepath"
)

// Source identifies where a resolved identity came from, for observability.
type Source string

const (
	SourceEnv        Source = "env"
	SourceCWDFile    Source = "cwd-file"
	SourceParentFile Source = "parent-file"
	SourceGlobalFile Source = "global-file"
)

// EnvVar is the environment override. Its value is used verbatim as the id — no
// file is consulted when it is set. This is the escape hatch for CI and scripted
// use where writing a file is undesirable.
const EnvVar = "JEFF_ORCHESTRATOR_ID"

// EnvVarLegacy is honored as a fallback for orchestrators launched by
// `jeff orchestrator start`, which export it into the pane shell. Keeping it
// means those existing sessions keep resolving without a migration step.
const EnvVarLegacy = "JEFF_ORCHESTRATOR_SESSION"

// detectParams are the ambient inputs the resolution chain depends on, injected
// so the chain can be unit-tested without touching the real cwd / $HOME / env.
type detectParams struct {
	getenv   func(string) string
	startDir string
	home     string
}

// Detect resolves the orchestrator identity id for the current process. It is a
// thin wrapper over detectWith that fills in the ambient os inputs.
func Detect() (string, Source, error) {
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()
	return detectWith(detectParams{getenv: os.Getenv, startDir: wd, home: home})
}

// detectWith implements the five-step resolution chain, most explicit/durable
// first:
//
//  1. $JEFF_ORCHESTRATOR_ID (or the legacy alias) — taken verbatim, no file lookup.
//  2. .jeff/orchestrator.json in the start directory.
//  3. .jeff/orchestrator.json in a parent directory (walking up, git-style,
//     stopping at $HOME so we never traverse into unrelated ancestors or /).
//  4. The machine-wide default ~/.jeff/default-orchestrator.json.
//  5. Not found.
//
// "Not found" is signalled by an empty id with a nil error, so callers decide
// whether the absence is fatal (crew start cannot proceed) or tolerable (crew
// list can show everything with --all). A non-nil error is returned ONLY for a
// genuine I/O error or a malformed file that exists — those must fail loud
// rather than degrade to the shared-default path that this whole change kills.
func detectWith(p detectParams) (string, Source, error) {
	// 1. Explicit env override wins over any file.
	if v := p.getenv(EnvVar); v != "" {
		return v, SourceEnv, nil
	}
	if v := p.getenv(EnvVarLegacy); v != "" {
		return v, SourceEnv, nil
	}

	// 2. Per-project file in the start directory.
	if id, err := readIfExists(ProjectFilePath(p.startDir)); err != nil {
		return "", "", err
	} else if id != nil {
		return id.ID, SourceCWDFile, nil
	}

	// 3. Walk parent directories (stopping at $HOME).
	if id, err := walkParents(p.startDir, p.home); err != nil {
		return "", "", err
	} else if id != nil {
		return id.ID, SourceParentFile, nil
	}

	// 4. Machine-wide default.
	if p.home != "" {
		if id, err := readIfExists(GlobalFilePath(p.home)); err != nil {
			return "", "", err
		} else if id != nil {
			return id.ID, SourceGlobalFile, nil
		}
	}

	// 5. No identity found.
	return "", "", nil
}

// walkParents checks each ancestor of startDir for a per-project identity file,
// from the immediate parent upward. It stops at $HOME (inclusive) so a stray
// ~/.jeff/orchestrator.json can bind a whole home tree but the walk never climbs
// into $HOME's parents or all the way to /. When $HOME is not an ancestor of
// startDir (e.g. a project on a separate volume), the walk terminates at the
// filesystem root without matching, and resolution falls through to the global
// default.
func walkParents(startDir, home string) (*Identity, error) {
	dir := startDir
	for {
		// At $HOME we stop before ascending further; startDir itself was already
		// checked by the caller (step 2), and $HOME is checked below on the pass
		// that reaches it.
		if home != "" && dir == home {
			return nil, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil // filesystem root
		}
		dir = parent
		id, err := readIfExists(ProjectFilePath(dir))
		if err != nil {
			return nil, err
		}
		if id != nil {
			return id, nil
		}
	}
}
