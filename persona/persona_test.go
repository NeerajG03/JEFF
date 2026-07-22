package persona

import (
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	names := Names()
	if len(names) < 5 {
		t.Fatalf("expected at least 5 personas, got %d: %v", len(names), names)
	}

	expected := map[string]bool{"dickson": false, "eric": false, "hardy": false, "jenko": false, "marlowe": false, "schmidt": false}
	for _, n := range names {
		expected[n] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing persona: %s", name)
		}
	}
}

// TestPersonaConsistency makes phantom/hidden personas impossible: every
// Names() entry must have a default model, a default agent, and a loadable
// template, and marlowe must not silently disappear from Names().
func TestPersonaConsistency(t *testing.T) {
	names := Names()

	foundMarlowe := false
	for _, name := range names {
		if name == "marlowe" {
			foundMarlowe = true
		}
		if DefaultModel(name) == "" {
			t.Errorf("persona %q has no DefaultModel", name)
		}
		if DefaultAgent(name) == "" {
			t.Errorf("persona %q has no DefaultAgent", name)
		}
		if _, err := Get(name); err != nil {
			t.Errorf("persona %q: Get failed: %v", name, err)
		}
	}
	if !foundMarlowe {
		t.Errorf("marlowe missing from Names(): %v", names)
	}
}

func TestGet(t *testing.T) {
	content, err := Get("jenko")
	if err != nil {
		t.Fatalf("get jenko: %v", err)
	}
	if !strings.Contains(content, "Jenko") {
		t.Error("jenko persona should mention 'Jenko'")
	}
}

func TestGetInvalid(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Error("expected error for invalid persona")
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid("dickson") {
		t.Error("dickson should be valid")
	}
	if IsValid("nonexistent") {
		t.Error("nonexistent should be invalid")
	}
}

func TestDescription(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"jenko", "the implementer and builder"},
		{"schmidt", "the investigator"},
		{"dickson", "the orchestrator and planner"},
		{"eric", "the researcher and analyst"},
		{"hardy", "the reviewer and quality checker"},
		{"nonexistent", ""},
	}
	for _, tc := range tests {
		got := Description(tc.name)
		if tc.want != "" && got != tc.want {
			t.Errorf("Description(%q) = %q, want %q", tc.name, got, tc.want)
		}
		if tc.want == "" && got != "" {
			t.Errorf("Description(%q) = %q, want empty", tc.name, got)
		}
	}
}

func TestNamesWithDescriptions(t *testing.T) {
	results := NamesWithDescriptions()
	if len(results) < 5 {
		t.Fatalf("expected at least 5 results, got %d", len(results))
	}
	// Each result should contain a tab separator with a description.
	for _, r := range results {
		if !strings.Contains(r, "\t") {
			t.Errorf("expected tab-separated description, got %q", r)
		}
	}
	// Spot check jenko.
	found := false
	for _, r := range results {
		if strings.HasPrefix(r, "jenko\t") && strings.Contains(r, "implementer") {
			found = true
			break
		}
	}
	if !found {
		t.Error("jenko should have 'implementer' in description")
	}
}

func TestMemoryHint(t *testing.T) {
	hint := MemoryHint("jenko")
	if hint == "" {
		t.Error("jenko should have a memory hint")
	}
	if !strings.Contains(hint, "Code style") {
		t.Error("jenko hint should mention code style")
	}

	hint = MemoryHint("schmidt")
	if !strings.Contains(hint, "Root cause") {
		t.Error("schmidt hint should mention root cause")
	}

	hint = MemoryHint("nonexistent")
	if hint != "" {
		t.Errorf("nonexistent should have empty hint, got %q", hint)
	}
}

func TestDefaultModel(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"jenko", "opus"},
		{"schmidt", "opus"},
		{"dickson", "opus"},
		{"eric", "sonnet"},
		{"hardy", "sonnet"},
		{"nonexistent", ""},
	}
	for _, tc := range tests {
		got := DefaultModel(tc.name)
		if got != tc.want {
			t.Errorf("DefaultModel(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
