package jeff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo represents a registered codebase.
type Repo struct {
	Name      string // short name (e.g., "backend")
	URL       string // clone URL
	Path      string // absolute path to the clone
	PostSetup string // post-setup script path (optional)
}

// AddRepo registers and clones a codebase into JEFF_HOME/repos/.
// If name is empty, it is derived from the URL.
func AddRepo(cfg *Config, url, name string) (*Repo, error) {
	if name == "" {
		name = repoNameFromURL(url)
	}
	if name == "" {
		return nil, fmt.Errorf("cannot derive repo name from URL %q, provide --name", url)
	}

	if _, exists := cfg.Repos[name]; exists {
		return nil, fmt.Errorf("repo %q already registered", name)
	}

	dest := filepath.Join(cfg.Home, "repos", name)
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("directory %s already exists", dest)
	}

	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone: %w", err)
	}

	cfg.Repos[name] = &RepoConfig{URL: url}
	if err := SaveConfig(cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	return &Repo{Name: name, URL: url, Path: dest}, nil
}

// RemoveRepo unregisters a codebase and optionally deletes the clone.
func RemoveRepo(cfg *Config, name string, deleteFiles bool) error {
	if _, exists := cfg.Repos[name]; !exists {
		return fmt.Errorf("repo %q not registered", name)
	}

	if deleteFiles {
		dest := filepath.Join(cfg.Home, "repos", name)
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("remove %s: %w", dest, err)
		}
	}

	delete(cfg.Repos, name)
	return SaveConfig(cfg)
}

// ListRepos returns all registered repos.
func ListRepos(cfg *Config) []*Repo {
	var repos []*Repo
	for name, rc := range cfg.Repos {
		repos = append(repos, &Repo{
			Name:      name,
			URL:       rc.URL,
			Path:      filepath.Join(cfg.Home, "repos", name),
			PostSetup: rc.PostSetup,
		})
	}
	return repos
}

// SetPostSetup sets the post-setup script for a repo.
func SetPostSetup(cfg *Config, repoName, scriptPath string) error {
	rc, exists := cfg.Repos[repoName]
	if !exists {
		return fmt.Errorf("repo %q not registered", repoName)
	}
	rc.PostSetup = scriptPath
	return SaveConfig(cfg)
}

// repoNameFromURL extracts a short name from a git URL.
// e.g., "https://github.com/org/backend.git" → "backend"
func repoNameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimRight(url, "/")
	if i := strings.LastIndex(url, "/"); i >= 0 {
		return url[i+1:]
	}
	if i := strings.LastIndex(url, ":"); i >= 0 {
		return url[i+1:]
	}
	return url
}
