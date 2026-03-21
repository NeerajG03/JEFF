package jeff

import (
	"testing"

	"github.com/NeerajG03/JEFF/internal/testutil"
)

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url, want string
	}{
		{"https://github.com/org/backend.git", "backend"},
		{"https://github.com/org/backend", "backend"},
		{"git@github.com:org/frontend.git", "frontend"},
		{"https://github.com/org/infra-config.git", "infra-config"},
		{"backend", "backend"},
	}
	for _, tt := range tests {
		got := repoNameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("repoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestListReposEmpty(t *testing.T) {
	cfg := &Config{Repos: make(map[string]*RepoConfig)}
	repos := ListRepos(cfg)
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestListRepos(t *testing.T) {
	cfg := &Config{
		Repos: map[string]*RepoConfig{
			"backend":  {URL: "https://github.com/org/backend.git"},
			"frontend": {URL: "https://github.com/org/frontend.git"},
		},
		Home: "/tmp/test-jeff",
	}
	repos := ListRepos(cfg)
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}
}

func TestAddRepoDuplicate(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{
		Repos: map[string]*RepoConfig{"backend": {URL: "https://github.com/org/backend.git"}},
		Home:  home,
	}
	_, err := AddRepo(cfg, "https://github.com/org/other.git", "backend")
	if err == nil {
		t.Error("expected error for duplicate repo name")
	}
}

func TestRemoveRepoNotRegistered(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}
	err := RemoveRepo(cfg, "nonexistent", false)
	if err == nil {
		t.Error("expected error for unregistered repo")
	}
}

func TestSyncRepoNotRegistered(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}
	_, err := SyncRepo(cfg, "nonexistent")
	if err == nil {
		t.Error("expected error for unregistered repo")
	}
}

func TestSyncRepoMissingDir(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{
		Repos: map[string]*RepoConfig{"backend": {URL: "https://example.com/backend.git"}},
		Home:  home,
	}
	_, err := SyncRepo(cfg, "backend")
	if err == nil {
		t.Error("expected error for missing repo dir")
	}
}

func TestSyncAllReposEmpty(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}
	results := SyncAllRepos(cfg)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSyncResultFields(t *testing.T) {
	r := &SyncResult{Name: "backend", Behind: 3, Updated: true}
	if r.Name != "backend" {
		t.Errorf("expected backend, got %s", r.Name)
	}
	if !r.Updated {
		t.Error("expected updated to be true")
	}
	if r.Behind != 3 {
		t.Errorf("expected 3 behind, got %d", r.Behind)
	}
}
