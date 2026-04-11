package hooks

import "testing"

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	h := &Hook{Name: "test-hook", Source: SourceHome, Event: "SessionStart", Matcher: "*"}
	r.Register(h)

	got := r.Get("test-hook")
	if got == nil {
		t.Fatal("expected hook, got nil")
	}
	if got.Name != "test-hook" {
		t.Fatalf("got name %q, want %q", got.Name, "test-hook")
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	if r.Get("missing") != nil {
		t.Fatal("expected nil for missing hook")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry()
	h := &Hook{Name: "dup", Source: SourceHome}
	r.Register(h)

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	r.Register(h)
}

func TestAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&Hook{Name: "beta", Source: SourceHome})
	r.Register(&Hook{Name: "alpha", Source: SourceTask})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("got %d hooks, want 2", len(all))
	}
	if all[0].Name != "alpha" || all[1].Name != "beta" {
		t.Fatalf("got [%s, %s], want sorted [alpha, beta]", all[0].Name, all[1].Name)
	}
}

func TestBySource(t *testing.T) {
	r := NewRegistry()
	r.Register(&Hook{Name: "home1", Source: SourceHome})
	r.Register(&Hook{Name: "task1", Source: SourceTask})
	r.Register(&Hook{Name: "home2", Source: SourceHome})

	home := r.BySource(SourceHome)
	if len(home) != 2 {
		t.Fatalf("got %d home hooks, want 2", len(home))
	}

	task := r.BySource(SourceTask)
	if len(task) != 1 {
		t.Fatalf("got %d task hooks, want 1", len(task))
	}
}

func TestNames(t *testing.T) {
	r := NewRegistry()
	r.Register(&Hook{Name: "z-hook", Source: SourceHome})
	r.Register(&Hook{Name: "a-hook", Source: SourceHome})

	names := r.Names()
	if len(names) != 2 || names[0] != "a-hook" || names[1] != "z-hook" {
		t.Fatalf("got %v, want [a-hook, z-hook]", names)
	}
}

func TestDefaultRegistryHasBuiltins(t *testing.T) {
	r := DefaultRegistry()

	expected := []string{"checkpoint-nudge", "crew-context", "gig-instructions", "gig-ready-tasks", "inbox-check", "jeff-instructions", "jeff-repos", "orchestrator-inbox", "session-capture", "task-commands", "task-context", "worker-heartbeat", "worker-stop"}
	names := r.Names()
	if len(names) != len(expected) {
		t.Fatalf("got %d hooks %v, want %d %v", len(names), names, len(expected), expected)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestEnabledForSource(t *testing.T) {
	r := NewRegistry()
	r.Register(&Hook{Name: "h1", Source: SourceHome})
	r.Register(&Hook{Name: "h2", Source: SourceHome})
	r.Register(&Hook{Name: "t1", Source: SourceTask})

	// nil config = all enabled for source
	enabled := EnabledForSource(nil, SourceHome, r)
	if len(enabled) != 2 || !enabled["h1"] || !enabled["h2"] {
		t.Fatalf("nil config: got %v, want h1+h2 enabled", enabled)
	}

	// explicit disable
	cfg := map[string]bool{"h1": false}
	enabled = EnabledForSource(cfg, SourceHome, r)
	if len(enabled) != 1 || !enabled["h2"] {
		t.Fatalf("h1 disabled: got %v, want only h2", enabled)
	}

	// doesn't include other sources
	enabled = EnabledForSource(nil, SourceTask, r)
	if len(enabled) != 1 || !enabled["t1"] {
		t.Fatalf("task source: got %v, want only t1", enabled)
	}
}
