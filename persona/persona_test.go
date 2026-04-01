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

	expected := map[string]bool{"dickson": false, "eric": false, "hardy": false, "jenko": false, "schmidt": false}
	for _, n := range names {
		expected[n] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing persona: %s", name)
		}
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
