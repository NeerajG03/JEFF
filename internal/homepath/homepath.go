// Package homepath converts between absolute paths and home-relative paths for
// anything persisted inside a JEFF home.
//
// A home used to be identified by its absolute path: registries and generated
// settings stored `/Users/me/.jeff/.personas/jenko` verbatim, so moving the
// directory broke persona registration, skill registration, and every agent hook
// (#84). Storing the path relative to the home instead makes the home portable by
// construction — the same bytes are correct wherever the directory lands.
//
// Paths that genuinely live outside the home (an externally-registered skill on
// another disk) stay absolute; they are not the home's to relocate.
package homepath

import (
	"path/filepath"
	"strings"
)

// Rel renders path for storage inside home: home-relative when it is inside the
// home, unchanged when it is not. Use it on every write.
//
//	Rel("/h/.jeff", "/h/.jeff/.skills/pr-review") → ".skills/pr-review"
//	Rel("/h/.jeff", "/opt/shared/skills/foo")     → "/opt/shared/skills/foo"
func Rel(home, path string) string {
	if home == "" || path == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || escapes(rel) {
		return path
	}
	return rel
}

// Abs resolves a stored path against home: relative entries are joined onto the
// home, absolute entries pass through. Use it on every read.
func Abs(home, stored string) string {
	if stored == "" || filepath.IsAbs(stored) {
		return stored
	}
	if home == "" {
		return stored
	}
	return filepath.Join(home, stored)
}

// Inside reports whether path lies within home. Used by `jeff doctor` to flag
// stored paths that escape the resolved home — the state a relocated install
// lands in when something stored an absolute path.
func Inside(home, path string) bool {
	if home == "" || path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		return true // relative entries are home-relative by definition
	}
	rel, err := filepath.Rel(home, path)
	return err == nil && !escapes(rel)
}

// escapes reports whether a relative path climbs out of its base.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
