package task

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/skill"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

// PickupOpts configures a task pickup.
type PickupOpts struct {
	TaskID         string
	Persona        string
	Repos          []string
	ReposReadonly  []string
	OrchestratorID string // hook context (crew workers); "" for foreground pickup
	Prompt         string // used by crew start after Pickup; unused by Pickup itself
	AgentOverride  jeff.AgentTool
}

// PickupResult reports the outcome of a pickup.
type PickupResult struct {
	TaskDir string
	Claimed bool // whether this call performed the claim (vs resumed/self-healed)
}

// Pickup claims a task (unless already claimed), builds its workspace and
// worktrees, wires hooks/skills/memory, and writes the task CLAUDE.md. It is
// idempotent and self-healing:
//
//   - closed/cancelled           → error (reopen first)
//   - in_progress + workspace     → resume: skip Claim (Claimed=false)
//   - in_progress + no workspace  → self-heal: rebuild without re-claiming
//     (Claimed=false) — this is the main line for remote workers, where the hub
//     arbitrates the claim and the worker never calls Claim itself.
//   - open/blocked/deferred       → claim (Claimed=true)
//
// If this call performed the claim and a subsequent hard-fail step errors, the
// claim is rolled back (status→open, assignee cleared) and the partial
// workspace removed, so a re-run starts clean.
//
// The caller owns the store's lifecycle and must have run jeff.EnsureAttrs
// beforehand (EnsureAttrs is a *gig.Store concern, outside the Store interface).
func Pickup(store Store, cfg *jeff.Config, opts PickupOpts) (*PickupResult, error) {
	t, err := store.Get(opts.TaskID)
	if err != nil {
		return nil, fmt.Errorf("task %s not found: %w", opts.TaskID, err)
	}

	// Decide claim vs resume vs self-heal from the current status.
	claimedHere := false
	switch t.Status {
	case gig.StatusClosed, gig.StatusCancelled:
		return nil, fmt.Errorf("task %s is %s — reopen it first (gig reopen)", opts.TaskID, t.Status)
	case gig.StatusInProgress:
		if _, werr := workspace.Open(cfg.Home, opts.TaskID); werr == nil {
			// Resume: claim already holds and the workspace exists. All steps
			// below are idempotent, so re-run them against the existing dir.
			fmt.Fprintf(os.Stderr, "Resuming %s: %s (already claimed, workspace exists)\n", opts.TaskID, t.Title)
		} else {
			// Self-heal: a previous pickup failed after Claim, leaving the task
			// in_progress with no workspace. Proceed without re-claiming — the
			// claim already holds (rollback would only fire if WE claimed).
			fmt.Fprintf(os.Stderr, "Self-heal %s: in_progress with no workspace — rebuilding without re-claim\n", opts.TaskID)
		}
	default: // open / blocked / deferred
		claimResult, cerr := store.Claim(opts.TaskID, "jeff")
		if cerr != nil {
			return nil, fmt.Errorf("claim: %w", cerr)
		}
		claimedHere = true
		fmt.Fprintf(os.Stderr, "Claimed %s: %s\n", opts.TaskID, t.Title)
		if claimResult.ParentProgressed {
			fmt.Fprintf(os.Stderr, "Parent %s → in_progress\n", claimResult.ParentID)
		}
	}

	// Rollback guard: un-claim only if THIS call performed the claim. A resume
	// or self-heal must never un-claim on failure — the claim predates us.
	success := false
	defer func() {
		if !success && claimedHere {
			rollbackPickup(store, cfg.Home, opts.TaskID)
		}
	}()

	// HARD FAIL: workspace.Create uses MkdirAll, so it is idempotent on an
	// existing dir (resume/self-heal reuse it) and fails only if a non-dir file
	// blocks the path.
	td, err := workspace.Create(cfg.Home, opts.TaskID, t.Title)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Workspace: %s\n", td.Path)

	allRepos := append([]string{}, opts.Repos...)
	allRepos = append(allRepos, opts.ReposReadonly...)
	if len(allRepos) > 0 {
		reposJSON, _ := json.Marshal(allRepos)
		// HARD FAIL: repos attr is load-bearing for teardown worktree cleanup.
		if err := store.SetAttr(opts.TaskID, jeff.AttrRepos, string(reposJSON)); err != nil {
			return nil, fmt.Errorf("set repos attr: %w", err)
		}
	}
	if opts.Persona != "" {
		if err := store.SetAttr(opts.TaskID, jeff.AttrPersona, opts.Persona); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: set persona attr: %v\n", err)
		}
	}
	if err := store.SetAttr(opts.TaskID, jeff.AttrTeamSize, "1"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: set team_size attr: %v\n", err)
	}

	taskJSON := buildTaskJSON(store, t)
	for _, repoName := range opts.Repos {
		rc := cfg.Repos[repoName]
		branch, err := resolveRepoBranch(rc, taskJSON, opts.TaskID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: branch name for %s: %v, using %s\n", repoName, err, opts.TaskID)
			branch = opts.TaskID
		}

		wtOpts := workspace.WorktreeOpts{
			JeffHome: cfg.Home,
			RepoName: repoName,
			Branch:   branch,
			TaskDir:  td.Path,
		}
		if rc != nil {
			wtOpts.BaseBranch = rc.BaseBranch
			wtOpts.PostSetup = rc.PostSetup
		}

		wtDir, err := workspace.WorktreeAdd(wtOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree for %s: %v\n", repoName, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "Worktree: %s → %s\n", repoName, wtDir)
	}

	for _, repoName := range opts.ReposReadonly {
		target, err := workspace.ReadonlyLink(cfg.Home, repoName, td.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: readonly link for %s: %v\n", repoName, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "Readonly: %s → %s\n", repoName, target)
	}

	if opts.Persona != "" {
		if err := memory.EnsurePersonaDir(cfg.Home, opts.Persona); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: persona memory: %v\n", err)
		}
	}
	for _, repoName := range allRepos {
		if err := memory.EnsureRepoDir(cfg.Home, repoName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: repo learnings: %v\n", err)
		}
	}

	// HARD FAIL: the task CLAUDE.md is the agent's entire context.
	if err := WriteClaudeMD(td.Path, cfg.Home, store, t, opts.Persona, allRepos); err != nil {
		return nil, fmt.Errorf("write task CLAUDE.md: %w", err)
	}

	var memScopes []string
	if opts.Persona != "" {
		if content, _ := memory.LoadPersonaMemory(cfg.Home, opts.Persona); content != "" {
			memScopes = append(memScopes, "persona:"+opts.Persona)
		}
	}
	for _, r := range allRepos {
		if content, _ := memory.LoadRepoLearnings(cfg.Home, r); content != "" {
			memScopes = append(memScopes, "repo:"+r)
		}
	}
	if len(memScopes) > 0 {
		if data, err := json.Marshal(memScopes); err == nil {
			if err := store.SetAttr(opts.TaskID, jeff.AttrMemoryLoaded, string(data)); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: set memory_loaded attr: %v\n", err)
			}
		}
	}

	// Append a readonly notice so the agent knows which repos it must not modify.
	if len(opts.ReposReadonly) > 0 {
		if err := appendReadonlyNote(td.Path, opts.ReposReadonly); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: readonly note: %v\n", err)
		}
	}

	// Inject memory addendum + suppress native memory for the active agent.
	// RunSessionStart is idempotent; the bash hook re-runs it on session resume.
	agentKind := string(opts.AgentOverride)
	if agentKind == "" {
		agentKind = string(cfg.Agent)
	}
	if err := hooks.RunSessionStart(cfg.Home, td.Path, opts.Persona, opts.TaskID, allRepos, agentKind); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: memory session-start: %v\n", err)
	}

	reg := hooks.DefaultRegistry()
	mgr := hooks.NewManager(reg)
	hctx := hooks.HookContext{
		JeffHome:           cfg.Home,
		TargetDir:          td.Path,
		GigHome:            cfg.GigHome,
		TaskID:             opts.TaskID,
		OrchestratorID:     opts.OrchestratorID,
		CheckpointPatterns: cfg.CheckpointPatterns,
		Persona:            opts.Persona,
		Repos:              allRepos,
	}
	// Install hooks for ALL registered agents so the workspace is ready
	// regardless of which agent launches (same pattern as context aliases).
	taskEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceTask, reg)
	if len(taskEnabled) > 0 {
		for _, agent := range jeff.RegisteredAgents() {
			p := jeff.GetProvider(agent)
			if p == nil {
				continue
			}
			if err := mgr.Sync(td.Path, taskEnabled, p.HookDeliveryKey(), hctx); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: task hooks (%s): %v\n", agent, err)
			}
		}
	}

	// Always alias .gemini/skills → .claude/skills before injecting skills,
	// regardless of whether the gemini agent is registered. Skills should be
	// in sync across agents, so gemini sessions see what claude sessions get.
	if err := jeffembed.EnsureGeminiSkillsAlias(td.Path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .gemini/skills: %v\n", err)
	}

	// Inject matching skills into ALL registered agent config dirs that support skills.
	// This ensures skills are available regardless of which agent launches in this workspace.
	mctx := &skill.MatchContext{
		Persona: opts.Persona,
		GigType: string(t.Type),
		Labels:  t.Labels,
	}
	var injectedNames []string
	injectedSet := make(map[string]bool)
	for _, agent := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(agent)
		if p == nil || p.SkillsSubdir() == "" {
			continue
		}
		names, err := skill.InjectMatchingTo(cfg.Home, td.Path, p.ConfigDir(), p.SkillsSubdir(), mctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skill injection (%s): %v\n", agent, err)
		}
		if len(injectedNames) == 0 {
			injectedNames = names
		}
		for _, n := range names {
			injectedSet[n] = true
		}
	}
	if len(injectedNames) > 0 {
		fmt.Fprintf(os.Stderr, "Skills: %s\n", strings.Join(injectedNames, ", "))
	}
	if len(injectedSet) > 0 {
		names := make([]string, 0, len(injectedSet))
		for n := range injectedSet {
			names = append(names, n)
		}
		sort.Strings(names)
		if data, err := json.Marshal(names); err == nil {
			if err := store.SetAttr(opts.TaskID, jeff.AttrSkillsLoaded, string(data)); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: set skills_loaded attr: %v\n", err)
			}
		}
	}

	success = true
	return &PickupResult{TaskDir: td.Path, Claimed: claimedHere}, nil
}

// rollbackPickup un-claims a task and removes its partial workspace after a
// hard failure in Pickup. gig has no Unclaim, so the status/assignee pair is the
// un-claim. All steps are best-effort (warn-and-continue) — rollback must not
// mask the original error.
//
// Deliberately NOT undone:
//   - worktrees under worktrees/<repo>/<branch> — expensive to recreate and
//     reused (idempotent) by the next attempt.
//   - parent auto-progression from Claim — a sibling task may legitimately be
//     in progress, so un-progressing the parent would corrupt its state.
func rollbackPickup(store Store, jeffHome, taskID string) {
	fmt.Fprintf(os.Stderr, "Rolling back partial pickup of %s (un-claiming)\n", taskID)
	if err := store.UpdateStatus(taskID, gig.StatusOpen, "jeff"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: rollback status: %v\n", err)
	}
	empty := ""
	if _, err := store.Update(taskID, gig.UpdateParams{Assignee: &empty}, "jeff"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: rollback assignee: %v\n", err)
	}
	if err := workspace.Remove(jeffHome, taskID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: rollback workspace: %v\n", err)
	}
}
