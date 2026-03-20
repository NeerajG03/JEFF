package hooks

import (
	"fmt"
	"sort"
)

// Registry holds all known hooks.
type Registry struct {
	hooks map[string]*Hook
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{hooks: make(map[string]*Hook)}
}

// Register adds a hook to the registry. Panics on duplicate name.
func (r *Registry) Register(h *Hook) {
	if _, exists := r.hooks[h.Name]; exists {
		panic(fmt.Sprintf("hooks: duplicate hook name %q", h.Name))
	}
	r.hooks[h.Name] = h
}

// Get returns the hook with the given name, or nil if not found.
func (r *Registry) Get(name string) *Hook {
	return r.hooks[name]
}

// All returns all registered hooks in sorted order.
func (r *Registry) All() []*Hook {
	out := make([]*Hook, 0, len(r.hooks))
	for _, h := range r.hooks {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// BySource returns all hooks matching the given source, sorted by name.
func (r *Registry) BySource(s Source) []*Hook {
	var out []*Hook
	for _, h := range r.hooks {
		if h.Source == s {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns sorted names of all registered hooks.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.hooks))
	for name := range r.hooks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultRegistry returns a registry pre-loaded with all built-in hooks.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, h := range builtinHooks() {
		r.Register(h)
	}
	return r
}
