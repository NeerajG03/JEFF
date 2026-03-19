package persona

import (
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	names := Names()
	if len(names) < 4 {
		t.Fatalf("expected at least 4 personas, got %d: %v", len(names), names)
	}

	expected := map[string]bool{"captain": false, "nerd": false, "jock": false, "scout": false}
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
	content, err := Get("jock")
	if err != nil {
		t.Fatalf("get jock: %v", err)
	}
	if !strings.Contains(content, "Jock") {
		t.Error("jock persona should mention 'Jock'")
	}
}

func TestGetInvalid(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Error("expected error for invalid persona")
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid("captain") {
		t.Error("captain should be valid")
	}
	if IsValid("nonexistent") {
		t.Error("nonexistent should be invalid")
	}
}
