package main

import (
	"bytes"
	"encoding/json"
	"runtime"
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

func mkInstall(m ...string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m)/2)
	for i := 0; i < len(m)-1; i += 2 {
		out[m[i]] = m[i+1]
	}
	return out
}

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
		deps    []depResult
		env     []envCheck
		want    int
	}{
		{
			name: "all ok",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
				{dep: dep{Required: false}, Status: statusMissing},
			},
			want: 2,
		},
		{
			name: "required missing",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusMissing},
			},
			want: 1,
		},
		{
			name: "required outdated",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusOutdated},
			},
			want: 1,
		},
		{
			name: "optional missing does not fail hard",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
				{dep: dep{Required: false}, Status: statusMissing},
				{dep: dep{Required: false}, Status: statusOutdated},
			},
			want: 2,
		},
		{
			name: "env required fail gives exit 1",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
			},
			env:  []envCheck{{Name: "jeff_initialized", Status: envFail, Required: true}},
			want: 1,
		},
		{
			name: "env warn only gives exit 2",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
			},
			env:  []envCheck{{Name: "gh_authenticated", Status: envWarn, Required: false}},
			want: 2,
		},
		{
			name: "all green gives exit 0",
			deps: []depResult{
				{dep: dep{Required: true}, Status: statusOK},
			},
			env:  []envCheck{{Name: "jeff_initialized", Status: envOK, Required: true}},
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeExitCode(tc.deps, tc.env)
			if got != tc.want {
				t.Errorf("exitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

// --- platform filter ---

func TestFilterDepsForPlatform(t *testing.T) {
	deps := []dep{
		{Name: "gig", Required: true},
		{Name: "tmux", Required: false},
		{Name: "terminal-notifier", Required: false, OnlyOn: []string{"darwin"}},
	}

	if runtime.GOOS == "darwin" {
		result := filterDepsForPlatform(deps, platformInfo{OS: "darwin"})
		if len(result) != 3 {
			t.Errorf("darwin: expected 3 deps, got %d", len(result))
		}
	} else {
		result := filterDepsForPlatform(deps, platformInfo{OS: "linux"})
		if len(result) != 2 {
			t.Errorf("linux: expected 2 deps (terminal-notifier filtered), got %d", len(result))
		}
		for _, d := range result {
			if d.Name == "terminal-notifier" {
				t.Error("terminal-notifier should be filtered on linux")
			}
		}
	}
}

// --- install hint selection ---

func TestInstallHint_Darwin(t *testing.T) {
	platform := platformInfo{OS: "darwin"}
	d := dep{
		Name: "tmux",
		Install: map[string]string{
			"darwin": "brew install tmux",
		},
	}
	hint := installHint(d.Name, d.Install, platform)
	if hint != "brew install tmux" {
		t.Errorf("hint = %q, want brew install tmux", hint)
	}
}

func TestInstallHint_LinuxDistro(t *testing.T) {
	platform := platformInfo{OS: "linux", Distro: "debian"}
	d := dep{
		Name: "git",
		Install: map[string]string{
			"darwin": "brew install git",
		},
	}
	hint := installHint(d.Name, d.Install, platform)
	if hint != "sudo apt install git" {
		t.Errorf("linux distro hint = %q, want sudo apt install git", hint)
	}
}

func TestInstallHint_LinuxFallback(t *testing.T) {
	platform := platformInfo{OS: "linux"}
	d := dep{
		Name: "gig",
		Install: map[string]string{
			"": "go install ...",
		},
	}
	hint := installHint(d.Name, d.Install, platform)
	if hint != "go install ..." {
		t.Errorf("linux fallback hint = %q, want go install ...", hint)
	}
}

func TestInstallHint_Empty(t *testing.T) {
	platform := platformInfo{OS: "darwin"}
	hint := installHint("", nil, platform)
	if hint != "" {
		t.Errorf("empty map => empty hint, got %q", hint)
	}
	hint = installHint("", map[string]string{}, platform)
	if hint != "" {
		t.Errorf("empty map => empty hint, got %q", hint)
	}
}

// --- JSON output V1 ---

func TestPrintDoctorJSONV1_Shape(t *testing.T) {
	depResults := []depResult{
		{dep: dep{Name: "tmux", Required: false, RequiredFor: "crew", Install: mkInstall("darwin", "brew install tmux")}, Status: statusOK, Version: "3.4"},
		{dep: dep{Name: "terminal-notifier", Required: false, Install: mkInstall("darwin", "brew install terminal-notifier")}, Status: statusMissing},
	}

	envResults := []envCheck{
		{Name: "jeff_initialized", Status: envOK, Required: true},
		{Name: "gh_authenticated", Status: envWarn, Fix: "gh auth login", Required: false},
	}

	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	platform := platformInfo{OS: "darwin"}
	printDoctorJSONV1(cmd, depResults, envResults, platform)

	var out jsonDoctorOutputV1
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, buf.String())
	}

	if out.Version != 1 {
		t.Errorf("version = %d, want 1", out.Version)
	}
	if !out.OK {
		t.Error("expected ok=true when no required deps failed")
	}
	if len(out.Deps) != 2 {
		t.Errorf("expected 2 deps, got %d", len(out.Deps))
	}
	if len(out.Environment) != 2 {
		t.Errorf("expected 2 env checks, got %d", len(out.Environment))
	}
	if out.Platform.OS != "darwin" {
		t.Errorf("platform.os = %q, want darwin", out.Platform.OS)
	}
	if len(out.Next) != 0 {
		t.Errorf("next should be empty when all ok, got %v", out.Next)
	}
}

func TestPrintDoctorJSONV1_OKFalseWhenRequiredFailed(t *testing.T) {
	depResults := []depResult{
		{dep: dep{Name: "tmux", Required: true, Install: mkInstall("darwin", "brew install tmux")}, Status: statusMissing},
	}
	envResults := []envCheck{
		{Name: "jeff_initialized", Status: envFail, Fix: "jeff init", Required: true},
	}
	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	platform := platformInfo{OS: "darwin"}
	printDoctorJSONV1(cmd, depResults, envResults, platform)

	var out jsonDoctorOutputV1
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if out.OK {
		t.Error("expected ok=false when required dep is missing")
	}
	if len(out.Next) == 0 {
		t.Error("expected non-zero next array when things fail")
	}
}

// --- next steps ordering ---

func TestBuildNextSteps_Ordered(t *testing.T) {
	depResults := []depResult{
		{dep: dep{Name: "jq", Required: true, Install: mkInstall("darwin", "brew install jq")}, Status: statusMissing},
	}
	envResults := []envCheck{
		{Name: "jeff_initialized", Status: envFail, Fix: "jeff init", Required: true},
		{Name: "gig_initialized", Status: envFail, Fix: "gig init --prefix myapp", Required: true},
		{Name: "gh_authenticated", Status: envWarn, Fix: "gh auth login", Required: false},
	}
	platform := platformInfo{OS: "darwin"}
	next := buildNextSteps(depResults, envResults, platform)

	if len(next) < 3 {
		t.Fatalf("expected >= 3 next steps, got %d: %v", len(next), next)
	}
	if next[0] != "jeff init" {
		t.Errorf("next[0] = %q, want jeff init", next[0])
	}
}

// --- platform detection ---

func TestDetectPlatform(t *testing.T) {
	p := detectPlatform()
	if p.OS == "" {
		t.Error("platform OS should not be empty")
	}
	if p.OS != runtime.GOOS {
		t.Errorf("platform.OS = %q, runtime.GOOS = %q", p.OS, runtime.GOOS)
	}
}

func TestPackageManager(t *testing.T) {
	cases := map[string]string{
		"debian":  "apt",
		"ubuntu":  "apt",
		"fedora":  "dnf",
		"arch":    "pacman",
		"manjaro": "pacman",
		"alpine":  "apk",
		"unknown": "",
	}
	for distro, want := range cases {
		got := packageManager(distro)
		if got != want {
			t.Errorf("packageManager(%q) = %q, want %q", distro, got, want)
		}
	}
}

// --- table output ---

func TestPrintDoctorTable_ContainsDeps(t *testing.T) {
	results := []depResult{
		{dep: dep{Name: "git", Required: true}, Status: statusOK, Version: "2.42.0"},
		{dep: dep{Name: "terminal-notifier", Required: false, Install: mkInstall("darwin", "brew install terminal-notifier")}, Status: statusMissing},
	}

	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	printDoctorTable(cmd, results, platformInfo{OS: "darwin"})
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

func TestPrintEnvironmentTable_ContainsEnv(t *testing.T) {
	results := []envCheck{
		{Name: "jeff_initialized", Status: envOK, Required: true},
		{Name: "gh_authenticated", Status: envWarn, Fix: "gh auth login", Required: false},
		{Name: "repos_configured", Status: envFail, Fix: "jeff repo add <url>", Required: true},
	}

	var buf bytes.Buffer
	cmd := doctorCmd()
	cmd.SetOut(&buf)

	printEnvironmentTable(cmd, results)
	out := buf.String()

	for _, name := range []string{"jeff_initialized", "gh_authenticated", "repos_configured"} {
		if !strings.Contains(out, name) {
			t.Errorf("env table output missing %q", name)
		}
	}
	if !strings.Contains(out, "gh auth login") {
		t.Error("env table output missing fix string")
	}
}

// --- getDoctorDeps includes gig as required ---

func TestGetDoctorDeps_ContainsGig(t *testing.T) {
	deps := getDoctorDeps()
	var gigDep *dep
	for i := range deps {
		if deps[i].Name == "gig" {
			gigDep = &deps[i]
			break
		}
	}
	if gigDep == nil {
		t.Fatal("gig not found in doctor deps")
	}
	if !gigDep.Required {
		t.Error("gig should be required")
	}
}

// --- tmux is not required ---

func TestGetDoctorDeps_TmuxNotRequired(t *testing.T) {
	deps := getDoctorDeps()
	for _, d := range deps {
		if d.Name == "tmux" {
			if d.Required {
				t.Error("tmux should NOT be required")
			}
			if d.RequiredFor != "crew" {
				t.Errorf("tmux RequiredFor = %q, want crew", d.RequiredFor)
			}
			return
		}
	}
	t.Fatal("tmux not found in doctor deps")
}
