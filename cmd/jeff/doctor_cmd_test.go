package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// --- parseVersion ---

func TestParseVersion(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"tmux 3.4", "3.4"},
		{"tmux 3.4a", "3.4"},
		{"git version 2.42.0", "2.42.0"},
		{"gh version 2.40.1 (2024-01-15)\nhttps://...", "2.40.1"},
		{"1.0.0", "1.0.0"},
		{"claude 1.2.3", "1.2.3"},
		{"no version here", ""},
		{"", ""},
	}

	for _, tc := range cases {
		got := parseVersion(tc.input)
		if got != tc.want {
			t.Errorf("parseVersion(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- versionLess ---

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.9", "3.0", true},
		{"3.0", "3.0", false},
		{"3.1", "3.0", false},
		{"2.42.0", "2.43.0", true},
		{"2.43.0", "2.42.0", false},
		{"3.4", "3.0", false},
		{"1.0", "2.0", true},
		{"10.0", "9.0", false},
		{"3.0.0", "3.0", false},
		{"3.0", "3.0.1", true},
	}

	for _, tc := range cases {
		got := versionLess(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// --- splitVersion ---

func TestSplitVersion(t *testing.T) {
	cases := []struct {
		input string
		want  []int
	}{
		{"3.4", []int{3, 4}},
		{"3.4a", []int{3, 4}},
		{"2.42.0", []int{2, 42, 0}},
		{"1.0.0-beta", []int{1, 0, 0}},
	}

	for _, tc := range cases {
		got := splitVersion(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitVersion(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i, v := range tc.want {
			if got[i] != v {
				t.Errorf("splitVersion(%q)[%d] = %d, want %d", tc.input, i, got[i], v)
			}
		}
	}
}

// --- checkDep ---

func TestCheckDep_Missing(t *testing.T) {
	d := dep{
		Name:   "definitely-not-a-real-binary-xyz123",
		Binary: "definitely-not-a-real-binary-xyz123",
	}
	r := checkDep(d)
	if r.Status != statusMissing {
		t.Errorf("status = %q, want %q", r.Status, statusMissing)
	}
}

func TestCheckDep_GitPresent(t *testing.T) {
	d := dep{
		Name:        "git",
		Required:    true,
		Binary:      "git",
		VersionArgs: []string{"--version"},
	}
	r := checkDep(d)
	if r.Status != statusOK {
		t.Errorf("expected git to be present, got status %q", r.Status)
	}
	if r.Version == "" {
		t.Error("expected non-empty version for git")
	}
}

func TestCheckDep_OutdatedMinVersion(t *testing.T) {
	// Use a very high min version to force outdated.
	d := dep{
		Name:        "git",
		Required:    true,
		Binary:      "git",
		VersionArgs: []string{"--version"},
		MinVersion:  "999.0",
	}
	r := checkDep(d)
	if r.Status != statusOutdated {
		t.Errorf("status = %q, want %q", r.Status, statusOutdated)
	}
}

// --- exit code logic ---

func TestAnyRequiredFailed(t *testing.T) {
	cases := []struct {
		name    string
		results []depResult
		want    bool
	}{
		{
			name: "all ok",
			results: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
				{dep: dep{Required: false}, Status: statusMissing},
			},
			want: false,
		},
		{
			name: "required missing",
			results: []depResult{
				{dep: dep{Required: true}, Status: statusMissing},
			},
			want: true,
		},
		{
			name: "required outdated",
			results: []depResult{
				{dep: dep{Required: true}, Status: statusOutdated},
			},
			want: true,
		},
		{
			name: "optional missing does not fail",
			results: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
				{dep: dep{Required: false}, Status: statusMissing},
				{dep: dep{Required: false}, Status: statusOutdated},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := false
			for _, r := range tc.results {
				if r.Required && r.Status != statusOK {
					got = true
					break
				}
			}
			if got != tc.want {
				t.Errorf("anyRequiredFailed = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- JSON output ---

func TestPrintDoctorJSON_Shape(t *testing.T) {
	results := []depResult{
		{dep: dep{Name: "tmux", Required: true, InstallCmd: "brew install tmux"}, Status: statusOK, Version: "3.4"},
		{dep: dep{Name: "terminal-notifier", Required: false, InstallCmd: "brew install terminal-notifier"}, Status: statusMissing},
	}

	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	if err := printDoctorJSON(cmd, results, false); err != nil {
		t.Fatalf("printDoctorJSON: %v", err)
	}

	var out jsonDoctorOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, buf.String())
	}

	if !out.OK {
		t.Error("expected ok=true when no required deps failed")
	}
	if len(out.Deps) != 2 {
		t.Errorf("expected 2 deps, got %d", len(out.Deps))
	}

	tmuxDep := out.Deps[0]
	if tmuxDep.Name != "tmux" {
		t.Errorf("deps[0].name = %q, want tmux", tmuxDep.Name)
	}
	if tmuxDep.Status != "ok" {
		t.Errorf("deps[0].status = %q, want ok", tmuxDep.Status)
	}
	if tmuxDep.Version != "3.4" {
		t.Errorf("deps[0].version = %q, want 3.4", tmuxDep.Version)
	}
	if tmuxDep.Install != "" {
		t.Errorf("deps[0].install should be empty for ok dep, got %q", tmuxDep.Install)
	}

	notifierDep := out.Deps[1]
	if notifierDep.Install == "" {
		t.Error("deps[1].install should be non-empty for missing dep")
	}
}

func TestPrintDoctorJSON_OKFalseWhenRequiredFailed(t *testing.T) {
	results := []depResult{
		{dep: dep{Name: "tmux", Required: true, InstallCmd: "brew install tmux"}, Status: statusMissing},
	}

	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	if err := printDoctorJSON(cmd, results, true); err != nil {
		t.Fatalf("printDoctorJSON: %v", err)
	}

	var out jsonDoctorOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if out.OK {
		t.Error("expected ok=false when required dep is missing")
	}
}

// --- table output ---

func TestPrintDoctorTable_ContainsDeps(t *testing.T) {
	results := []depResult{
		{dep: dep{Name: "git", Required: true}, Status: statusOK, Version: "2.42.0"},
		{dep: dep{Name: "terminal-notifier", Required: false, InstallCmd: "brew install terminal-notifier"}, Status: statusMissing},
	}

	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	printDoctorTable(cmd, results)
	out := buf.String()

	if !strings.Contains(out, "git") {
		t.Error("table output missing 'git'")
	}
	if !strings.Contains(out, "terminal-notifier") {
		t.Error("table output missing 'terminal-notifier'")
	}
	if !strings.Contains(out, "brew install terminal-notifier") {
		t.Error("table output missing install command for terminal-notifier")
	}
	if !strings.Contains(out, "2.42.0") {
		t.Error("table output missing version")
	}
}
