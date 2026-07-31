// home.go — the JEFF_HOME lifecycle.
//
// There are exactly two operations on a home, and they must not be confused:
//
//	SELECTION  — "where should a home live?"  Asked ONCE, by `jeff init` (and by
//	             `jeff home use`). It is the only operation permitted to write the
//	             pointer file.
//	RESOLUTION — "where is my home?"  Asked by every other command, on every run.
//	             It is READ-ONLY. It must never write anything, anywhere.
//
// Collapsing those two is what produced #82 (init ignored JEFF_HOME and rewrote
// the pointer to its own hardcoded default) and the unfiled clobber bug where a
// one-shot `JEFF_HOME=/tmp/x jeff status` permanently repointed the pointer file,
// silently promoting the most transient layer into the most durable one.
//
// The three resolution layers are three SCOPES, not three ways to do one thing:
//
//	env JEFF_HOME          per-process / per-tmux-session. Load-bearing for crew:
//	                       crew.Start does `tmux set-environment JEFF_HOME` so every
//	                       pane inherits one home and workers cannot drift onto a
//	                       different jeff.db.
//	~/.config/jeff/home    per-user, persistent. The install record — the only thing
//	                       that knows a non-default home exists.
//	~/.jeff                bootstrap default, for the very first run.
package jeff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvHome is the environment variable that overrides home resolution for a
// single process (and, via tmux set-environment, for a crew session).
const EnvHome = "JEFF_HOME"

// defaultHomeDirName is the bootstrap home directory under $HOME.
const defaultHomeDirName = ".jeff"

// HomeSource records which layer of the chain produced a home path. Returned
// alongside the path so commands (and `jeff doctor`) can explain *why* a home was
// chosen — every bug in this area was hard to see precisely because the provenance
// was invisible.
type HomeSource string

const (
	// HomeSourceFlag is an explicit CLI choice (`jeff init --home`/`--here`).
	HomeSourceFlag HomeSource = "flag"
	// HomeSourceEnv is the JEFF_HOME environment variable.
	HomeSourceEnv HomeSource = "env"
	// HomeSourcePointer is the ~/.config/jeff/home pointer file.
	HomeSourcePointer HomeSource = "pointer"
	// HomeSourceDefault is the ~/.jeff bootstrap default.
	HomeSourceDefault HomeSource = "default"
)

// Describe renders a human-readable explanation of a source, for doctor output
// and error messages.
func (s HomeSource) Describe() string {
	switch s {
	case HomeSourceFlag:
		return "explicit flag"
	case HomeSourceEnv:
		return "$" + EnvHome
	case HomeSourcePointer:
		return "pointer file (" + pointerPathOrLabel() + ")"
	case HomeSourceDefault:
		return "default (~/" + defaultHomeDirName + ")"
	}
	return string(s)
}

// PointerPath returns ~/.config/jeff/home, the file that records where a
// non-default home lives.
func PointerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".config", "jeff", "home"), nil
}

// pointerPathOrLabel is PointerPath for display, degrading to a generic label
// rather than failing a Describe() call.
func pointerPathOrLabel() string {
	if p, err := PointerPath(); err == nil {
		return p
	}
	return "~/.config/jeff/home"
}

// DefaultHome returns the bootstrap home path, ~/.jeff.
func DefaultHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, defaultHomeDirName), nil
}

// ResolveHomeWithSource answers "where is my home?" and reports which layer
// decided. Precedence: $JEFF_HOME → pointer file → ~/.jeff.
//
// This function is READ-ONLY by contract. It must never create or modify a file —
// in particular it must never write the pointer. TestResolveHomeNeverWrites pins
// that guarantee.
func ResolveHomeWithSource() (string, HomeSource, error) {
	if env := strings.TrimSpace(os.Getenv(EnvHome)); env != "" {
		// Normalize: a relative $JEFF_HOME would otherwise be re-anchored to
		// whatever the cwd happens to be at each use, and homepath.Abs would join
		// registry entries onto a relative base. Absolute here, once.
		if abs, err := filepath.Abs(env); err == nil {
			return abs, HomeSourceEnv, nil
		}
		return env, HomeSourceEnv, nil
	}

	if ptr, err := PointerPath(); err == nil {
		if data, err := os.ReadFile(ptr); err == nil {
			if p := strings.TrimSpace(string(data)); p != "" {
				return p, HomeSourcePointer, nil
			}
		}
	}

	def, err := DefaultHome()
	if err != nil {
		return "", "", err
	}
	return def, HomeSourceDefault, nil
}

// ResolveHome answers "where is my home?" without the provenance. Read-only, as
// ResolveHomeWithSource is.
func ResolveHome() (string, error) {
	home, _, err := ResolveHomeWithSource()
	return home, err
}

// SelectHomeOpts carries the explicit intent available to the selection path.
type SelectHomeOpts struct {
	// Explicit is a user-supplied path (`jeff init --home <path>`). Highest
	// precedence: the user named a directory, so nothing may override it.
	Explicit string
	// Here requests ./jeff in the current working directory (`jeff init --here`).
	Here bool
}

// SelectHomeForInit answers "where should a NEW home live?" — the write-path
// counterpart to ResolveHomeWithSource, and the only selector `jeff init` may use.
//
// Precedence: --home → --here → $JEFF_HOME → pointer file → ~/.jeff.
//
// Explicit flags outrank the ambient chain because they are a direct instruction;
// everything below them mirrors resolution exactly, so `jeff init` can never again
// disagree with the command that runs next about where the home is (#82).
func SelectHomeForInit(opts SelectHomeOpts) (string, HomeSource, error) {
	if p := strings.TrimSpace(opts.Explicit); p != "" {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", "", fmt.Errorf("resolve --home path: %w", err)
		}
		return abs, HomeSourceFlag, nil
	}

	if opts.Here {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("get cwd: %w", err)
		}
		return filepath.Join(cwd, "jeff"), HomeSourceFlag, nil
	}

	return ResolveHomeWithSource()
}

// WriteHomePointer records jeffHome in the pointer file. Callers are limited to
// the SELECTION path — `jeff init` and `jeff home use`. Never call this from a
// read path: doing so lets a transient $JEFF_HOME override rewrite the persistent
// default for every future shell.
func WriteHomePointer(jeffHome string) error {
	ptr, err := PointerPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ptr), 0o755); err != nil {
		return fmt.Errorf("create pointer dir: %w", err)
	}
	return os.WriteFile(ptr, []byte(jeffHome+"\n"), 0o644)
}

// IsHomeInitialized reports whether jeffHome holds an initialized JEFF install,
// i.e. whether jeff.json exists there.
func IsHomeInitialized(jeffHome string) bool {
	if jeffHome == "" {
		return false
	}
	_, err := os.Stat(ConfigPath(jeffHome))
	return err == nil
}
