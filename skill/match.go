package skill

import (
	"slices"
	"sort"
)

// MatchContext holds the task context used for skill matching.
type MatchContext struct {
	Persona string   // active persona name (from --persona flag)
	GigType string   // task type from gig (bug, feature, etc.)
	Labels  []string // task labels from gig
}

// Match returns true if any non-empty dimension of the skill entry matches
// the context. All-empty dimensions means the skill is manual-only.
func Match(entry *SkillEntry, ctx *MatchContext) bool {
	if len(entry.Personas) > 0 && ctx.Persona != "" {
		if slices.Contains(entry.Personas, ctx.Persona) {
			return true
		}
	}
	if len(entry.GigTypes) > 0 && ctx.GigType != "" {
		if slices.Contains(entry.GigTypes, ctx.GigType) {
			return true
		}
	}
	if len(entry.Tags) > 0 && len(ctx.Labels) > 0 {
		if intersects(entry.Tags, ctx.Labels) {
			return true
		}
	}
	return false
}

// MatchAll returns the sorted names of all skills that match the context.
func MatchAll(sc *SkillConfig, ctx *MatchContext) []string {
	var names []string
	for name, entry := range sc.Skills {
		if Match(entry, ctx) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// intersects returns true if any element appears in both slices.
func intersects(a, b []string) bool {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	for _, s := range a {
		if set[s] {
			return true
		}
	}
	return false
}
