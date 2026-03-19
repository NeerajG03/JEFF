package jeff

import "testing"

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
	home := tempHome(t)
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
	home := tempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}
	err := RemoveRepo(cfg, "nonexistent", false)
	if err == nil {
		t.Error("expected error for unregistered repo")
	}
}
