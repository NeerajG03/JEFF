package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

// setupRepo creates a minimal real git repo under home/repos/<name> on branch
// main (no remote, so WorktreeAdd skips the fetch step).
func setupRepo(t *testing.T, home, name string) {
	t.Helper()
	repoDir := filepath.Join(home, "repos", name)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hi\n"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "init")
}

// fixture builds a temp JEFF_HOME, a repos/<repo> git repo, an in-memory gig
// store with attrs registered, and a cfg pointing at them. Returns the store,
// cfg, and repo name.
func fixture(t *testing.T, withRepo bool) (*gig.Store, *jeff.Config, string) {
	t.Helper()
	home := t.TempDir()
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	cfg := &jeff.Config{Home: home, Agent: jeff.AgentClaudeCode, Repos: map[string]*jeff.RepoConfig{}}
	repo := "backend"
	if withRepo {
		setupRepo(t, home, repo)
		cfg.Repos[repo] = &jeff.RepoConfig{BaseBranch: "main"}
	}
	return store, cfg, repo
}

func newOpenTask(t *testing.T, store *gig.Store, title string) *gig.Task {
	t.Helper()
	task, err := store.Create(gig.CreateParams{Title: title, Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestPickup_HappyPath(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Happy path")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Persona: "jenko", Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	if !res.Claimed {
		t.Error("expected Claimed=true on fresh pickup")
	}

	// Task is in_progress, assigned to jeff.
	got, _ := store.Get(task.ID)
	if got.Status != gig.StatusInProgress {
		t.Errorf("expected in_progress, got %s", got.Status)
	}
	if got.Assignee != "jeff" {
		t.Errorf("expected assignee jeff, got %q", got.Assignee)
	}

	// Workspace + CLAUDE.md exist.
	if _, err := os.Stat(filepath.Join(res.TaskDir, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md missing: %v", err)
	}

	// Worktree symlink resolves.
	wts, err := workspace.ListTaskWorktrees(res.TaskDir)
	if err != nil || len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d (%v)", len(wts), err)
	}
	if wts[0].Repo != repo {
		t.Errorf("expected repo %s, got %q", repo, wts[0].Repo)
	}

	// Repos attr recorded.
	if attr, err := store.GetAttr(task.ID, jeff.AttrRepos); err != nil || attr == nil {
		t.Errorf("repos attr not set: %v", err)
	}
}

func TestPickup_IdempotentResume(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Resume")

	first, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("first Pickup: %v", err)
	}

	// Second call resumes: no error, Claimed=false, workspace intact.
	second, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("resume Pickup: %v", err)
	}
	if second.Claimed {
		t.Error("expected Claimed=false on resume")
	}
	if second.TaskDir != first.TaskDir {
		t.Errorf("resume TaskDir %q != first %q", second.TaskDir, first.TaskDir)
	}
	if _, err := os.Stat(filepath.Join(second.TaskDir, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md missing after resume: %v", err)
	}
}

func TestPickup_RollbackOnHardFailure(t *testing.T) {
	store, cfg, _ := fixture(t, false)
	task := newOpenTask(t, store, "Rollback")

	// Force a hard failure in workspace.Create: make home/tasks a regular file
	// so MkdirAll(home/tasks/<slug>) fails with "not a directory".
	if err := os.WriteFile(filepath.Join(cfg.Home, "tasks"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID})
	if err == nil {
		t.Fatal("expected Pickup to fail")
	}

	// Rollback un-claimed the task: back to open with empty assignee.
	got, _ := store.Get(task.ID)
	if got.Status != gig.StatusOpen {
		t.Errorf("expected rollback to open, got %s", got.Status)
	}
	if got.Assignee != "" {
		t.Errorf("expected empty assignee after rollback, got %q", got.Assignee)
	}
}

func TestPickup_SelfHealAlreadyClaimed(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Self heal")

	// Simulate a previous pickup that claimed but built no workspace.
	if _, err := store.Claim(task.ID, "jeff"); err != nil {
		t.Fatal(err)
	}

	// Pickup must self-heal (no double-claim error) and build the workspace.
	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("self-heal Pickup: %v", err)
	}
	if res.Claimed {
		t.Error("expected Claimed=false when task was pre-claimed")
	}
	if _, err := os.Stat(filepath.Join(res.TaskDir, "CLAUDE.md")); err != nil {
		t.Errorf("workspace not built on self-heal: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.Status != gig.StatusInProgress {
		t.Errorf("expected in_progress, got %s", got.Status)
	}
}

func TestPickup_TerminalStatusErrors(t *testing.T) {
	store, cfg, _ := fixture(t, false)
	task := newOpenTask(t, store, "Terminal")
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	_, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID})
	if err == nil {
		t.Fatal("expected error for terminal task")
	}
	if !strings.Contains(err.Error(), "reopen") {
		t.Errorf("expected error mentioning reopen, got: %v", err)
	}
}

func TestPickup_HotfixBranchInference(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	cfg.Repos[repo].BaseBranch = "production"

	repoDir := filepath.Join(cfg.Home, "repos", repo)
	cmd := exec.Command("git", "-C", repoDir, "branch", "production")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch production: %v\n%s", err, out)
	}

	task := newOpenTask(t, store, "Hotfix task")
	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	wts, err := workspace.ListTaskWorktrees(res.TaskDir)
	if err != nil || len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d (%v)", len(wts), err)
	}

	wantBranch := "hotfix/" + task.ID
	if wts[0].Branch != wantBranch {
		t.Errorf("expected hotfix branch %q, got %q", wantBranch, wts[0].Branch)
	}
}
