package jeff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF/internal/gitutil"
)

// Repo represents a registered codebase.
type Repo struct {
	Name        string // short name (e.g., "backend")
	URL         string // clone URL
	Description string // human-readable description (optional)
	Path        string // absolute path to the clone
	PostSetup   string // post-setup script path (optional)
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

	if err := gitutil.Run(".", "clone", url, dest); err != nil {
		return nil, err
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
			Name:        name,
			URL:         rc.URL,
			Description: rc.Description,
			Path:        filepath.Join(cfg.Home, "repos", name),
			PostSetup:   rc.PostSetup,
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

// SetDescription sets the description for a repo.
func SetDescription(cfg *Config, repoName, description string) error {
	rc, exists := cfg.Repos[repoName]
	if !exists {
		return fmt.Errorf("repo %q not registered", repoName)
	}
	rc.Description = description
	return SaveConfig(cfg)
}

// SyncResult holds the outcome of syncing a repo.
type SyncResult struct {
	Name    string
	Behind  int
	Updated bool
	Err     error
}

// SyncRepo pulls latest main from origin for a single repo.
func SyncRepo(cfg *Config, name string) (*SyncResult, error) {
	result := &SyncResult{Name: name}

	if _, exists := cfg.Repos[name]; !exists {
		return nil, fmt.Errorf("repo %q not registered", name)
	}

	repoDir := filepath.Join(cfg.Home, "repos", name)
	if _, err := os.Stat(repoDir); err != nil {
		return nil, fmt.Errorf("repo dir not found: %s", repoDir)
	}

	// Fetch latest from origin.
	if _, err := gitutil.Output(repoDir, "fetch", "origin"); err != nil {
		return nil, fmt.Errorf("git fetch %s: %w", name, err)
	}

	// Check how many commits behind.
	countOut, err := gitutil.Output(repoDir, "rev-list", "--count", "HEAD..origin/main")
	if err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &result.Behind)
	}

	if result.Behind == 0 {
		return result, nil
	}

	// Checkout main and fast-forward.
	_ = gitutil.Run(repoDir, "checkout", "main") // ignore if already on main

	if _, err := gitutil.Output(repoDir, "pull", "origin", "main", "--ff-only"); err != nil {
		return nil, fmt.Errorf("git pull %s: %w", name, err)
	}

	result.Updated = true
	return result, nil
}

// SyncAllRepos pulls latest main for all registered repos.
func SyncAllRepos(cfg *Config) []*SyncResult {
	var results []*SyncResult
	for name := range cfg.Repos {
		r, err := SyncRepo(cfg, name)
		if err != nil {
			results = append(results, &SyncResult{Name: name, Err: err})
		} else {
			results = append(results, r)
		}
	}
	return results
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
