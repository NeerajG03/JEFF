package homepath

import (
	"path/filepath"
	"testing"
)

func TestRel(t *testing.T) {
	home := filepath.FromSlash("/h/.jeff")
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"inside the home becomes relative", filepath.FromSlash("/h/.jeff/.skills/pr-review"), filepath.FromSlash(".skills/pr-review")},
		{"the home itself becomes dot", home, "."},
		{"outside the home stays absolute", filepath.FromSlash("/opt/shared/skills/foo"), filepath.FromSlash("/opt/shared/skills/foo")},
		{"a sibling that shares a prefix stays absolute", filepath.FromSlash("/h/.jeff-old/.skills/x"), filepath.FromSlash("/h/.jeff-old/.skills/x")},
		{"already-relative is untouched", filepath.FromSlash(".skills/x"), filepath.FromSlash(".skills/x")},
		{"empty is untouched", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Rel(home, tt.in); got != tt.want {
				t.Errorf("Rel(%q, %q) = %q, want %q", home, tt.in, got, tt.want)
			}
		})
	}
}

func TestAbs(t *testing.T) {
	home := filepath.FromSlash("/h/.jeff")
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"relative resolves against the home", filepath.FromSlash(".personas/jenko"), filepath.FromSlash("/h/.jeff/.personas/jenko")},
		{"absolute passes through", filepath.FromSlash("/opt/shared/x"), filepath.FromSlash("/opt/shared/x")},
		{"empty passes through", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Abs(home, tt.in); got != tt.want {
				t.Errorf("Abs(%q, %q) = %q, want %q", home, tt.in, got, tt.want)
			}
		})
	}
}

// TestRelAbsRoundTrip is the property that makes a home relocatable: what Rel
// stores, Abs resolves back — against ANY home, not just the one it was written
// under. That is exactly what a moved home needs.
func TestRelAbsRoundTrip(t *testing.T) {
	oldHome := filepath.FromSlash("/old/place/.jeff")
	newHome := filepath.FromSlash("/new/place/jeff")
	original := filepath.Join(oldHome, ".personas", "jenko")

	stored := Rel(oldHome, original)
	if filepath.IsAbs(stored) {
		t.Fatalf("Rel kept an absolute path (%q); the home would not be relocatable", stored)
	}

	if got, want := Abs(oldHome, stored), original; got != want {
		t.Errorf("round trip under the original home = %q, want %q", got, want)
	}
	if got, want := Abs(newHome, stored), filepath.Join(newHome, ".personas", "jenko"); got != want {
		t.Errorf("round trip under a MOVED home = %q, want %q", got, want)
	}
}

func TestInside(t *testing.T) {
	home := filepath.FromSlash("/h/.jeff")
	cases := []struct {
		path string
		want bool
	}{
		{filepath.FromSlash("/h/.jeff/.skills/x"), true},
		{filepath.FromSlash(".skills/x"), true}, // relative is home-relative by definition
		{filepath.FromSlash("/h/.jeff"), true},
		{filepath.FromSlash("/h/other"), false},
		{filepath.FromSlash("/h/.jeff-old/x"), false},
		{"", false},
	}
	for _, c := range cases {
		if got := Inside(home, c.path); got != c.want {
			t.Errorf("Inside(%q, %q) = %v, want %v", home, c.path, got, c.want)
		}
	}
}
