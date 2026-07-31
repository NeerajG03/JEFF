package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/skill"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type initOpts struct {
	home      string
	here      bool
	yes       bool
	agent     string
	ide       string
	gigPrefix string
	noRepos   bool
	verbose   bool
	repos     []string
}

// exitCode is an error that carries a specific process exit code.
type exitCode struct {
	code int
	msg  string
}

func (e *exitCode) Error() string { return e.msg }

func initCmd() *cobra.Command {
	var opts initOpts
	var update bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize JEFF home directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if update {
				return runUpdate()
			}
			err := runInit(cmd, &opts)
			if err != nil {
				var ece *exitCode
				if errors.As(err, &ece) {
					os.Exit(ece.code)
				}
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.home, "home", "", "Initialize the home at this exact path (highest precedence)")
	cmd.Flags().BoolVar(&opts.here, "here", false, "Initialize in current directory instead of ~/.jeff/")
	cmd.Flags().BoolVar(&update, "update", false, "Sync existing home (create missing dirs, hooks, settings)")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Skip wizard, accept defaults (escape hatch for CI)")
	cmd.Flags().StringVar(&opts.agent, "agent", "", "Agent CLI to use (claude, opencode, gemini)")
	cmd.Flags().StringVar(&opts.ide, "ide", "", "IDE to use (vscode, cursor, windsurf, nvim, zed)")
	cmd.Flags().StringArrayVar(&opts.repos, "repo", nil, "Repo to add (name=url, repeatable)")
	cmd.Flags().BoolVar(&opts.noRepos, "no-repos", false, "Skip repo setup")
	cmd.Flags().StringVar(&opts.gigPrefix, "gig-prefix", "", "Custom gig task ID prefix")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Show detailed output")
	cmd.MarkFlagsMutuallyExclusive("here", "update")
	return cmd
}

// ---------------------------------------------------------------------------
// C2: Agent CLI detection via exec.LookPath
// ---------------------------------------------------------------------------

func detectInstalledAgents() map[jeff.AgentTool]bool {
	installed := make(map[jeff.AgentTool]bool)
	for _, a := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(a)
		if p == nil {
			continue
		}
		_, err := exec.LookPath(p.Command())
		installed[a] = err == nil
	}
	return installed
}

// defaultAgentFromInstalled picks the best default agent from what's on PATH.
// Preference: claude > opencode > gemini. None found → empty (no default).
func defaultAgentFromInstalled() jeff.AgentTool {
	installed := detectInstalledAgents()
	preferred := []jeff.AgentTool{jeff.AgentClaudeCode, jeff.AgentOpenCode, jeff.AgentGemini}
	for _, a := range preferred {
		if installed[a] {
			return a
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// C1: Interactive wizard + silent flag-driven mode
// ---------------------------------------------------------------------------

// resolveHome delegates to the SDK selection path rather than reimplementing the
// precedence chain. It used to keep its own copy, which is how `jeff init` drifted
// out of agreement with every other command about where the home is (#82): the
// copy grew a `--here` branch and a $HOME/.jeff default but never learned about
// the pointer file. One selector, one precedence, no drift.
func resolveHome(opts *initOpts) (string, jeff.HomeSource, error) {
	return jeff.SelectHomeForInit(jeff.SelectHomeOpts{
		Explicit: opts.home,
		Here:     opts.here,
	})
}

func checkExisting(home string) error {
	existing, err := jeff.ResolveHome()
	if err == nil && existing != home {
		if _, err2 := os.Stat(jeff.ConfigPath(existing)); err2 == nil {
			return fmt.Errorf("JEFF is already initialized at %s\nRun `jeff init --update` to sync, or remove the pointer: rm ~/.config/jeff/home", existing)
		}
	}
	if _, err := os.Stat(jeff.ConfigPath(home)); err == nil {
		return fmt.Errorf("JEFF is already initialized at %s\nRun `jeff init --update` to sync", home)
	}
	return nil
}

// homePathLabel renders the home for the generated CLAUDE.md. It reports the
// path actually selected instead of guessing from flags, so a home placed by
// $JEFF_HOME or --home is not documented as "~/.jeff/".
func homePathLabel(home string, here bool) string {
	if here {
		return "jeff/"
	}
	if def, err := jeff.DefaultHome(); err == nil && home == def {
		return "~/.jeff/"
	}
	return home
}

// runInitWizard prompts the user interactively to pick agent, IDE, and repos.
func runInitWizard(cmd *cobra.Command, opts *initOpts) {
	installed := detectInstalledAgents()
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, colorize(cBold, "JEFF Setup Wizard"))
	fmt.Fprintln(out, strings.Repeat("─", 40))
	fmt.Fprintln(out)

	// --- Agent selection ---
	fmt.Fprintln(out, "Detected agent CLIs:")
	for _, a := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(a)
		if p == nil {
			continue
		}
		if installed[a] {
			fmt.Fprintf(out, "  %s %s\n", colorize(cGreen, "✓"), string(a))
		} else {
			fmt.Fprintf(out, "  %s %s\n", colorize(cRed, "✗"), string(a)+" (not installed)")
		}
	}
	fmt.Fprintln(out)

	defaultAgent := defaultAgentFromInstalled()
	agentNames := jeff.RegisteredAgents()
	nameStrs := make([]string, 0, len(agentNames))
	for _, a := range agentNames {
		nameStrs = append(nameStrs, string(a))
	}

	var defaultPrompt string
	if defaultAgent == "" {
		defaultPrompt = "(no agent CLI detected — pick one)"
	} else {
		defaultPrompt = string(defaultAgent)
	}

	for {
		fmt.Fprintf(out, "Which agent would you like to use? [%s] (default: %s): ", strings.Join(nameStrs, "/"), defaultPrompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			if defaultAgent != "" {
				input = string(defaultAgent)
			} else {
				continue
			}
		}
		if jeff.AgentTool(input).IsValid() {
			opts.agent = input
			break
		}
		fmt.Fprintf(out, "Invalid agent. Choose from: %s\n", strings.Join(nameStrs, ", "))
	}

	// --- IDE selection ---
	validIDEs := jeff.ValidIDEs
	ideStrs := make([]string, 0, len(validIDEs))
	for _, ide := range validIDEs {
		ideStrs = append(ideStrs, string(ide))
	}
	ideDefault := string(jeff.IDEVSCode)
	for {
		fmt.Fprintf(out, "Which IDE do you use? [%s] (default: %s): ", strings.Join(ideStrs, "/"), ideDefault)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			input = ideDefault
		}
		if jeff.IDE(input).IsValid() {
			opts.ide = input
			break
		}
		fmt.Fprintf(out, "Invalid IDE. Choose from: %s\n", strings.Join(ideStrs, ", "))
	}

	// --- Repo setup ---
	fmt.Fprint(out, "Add a repository now? [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "y" || input == "yes" {
		for {
			fmt.Fprint(out, "  Repository name (e.g., \"backend\"): ")
			name, _ := reader.ReadString('\n')
			name = strings.TrimSpace(name)
			if name == "" {
				break
			}
			fmt.Fprint(out, "  Repository URL (leave blank to add later): ")
			url, _ := reader.ReadString('\n')
			url = strings.TrimSpace(url)
			if url != "" {
				opts.repos = append(opts.repos, name+"="+url)
			} else {
				opts.repos = append(opts.repos, name+"=")
			}
			fmt.Fprint(out, "Add another repository? [y/N]: ")
			again, _ := reader.ReadString('\n')
			again = strings.TrimSpace(strings.ToLower(again))
			if again != "y" && again != "yes" {
				break
			}
		}
	}
	opts.noRepos = len(opts.repos) == 0
}

func runInit(cmd *cobra.Command, opts *initOpts) error {
	home, homeSource, err := resolveHome(opts)
	if err != nil {
		return err
	}
	if opts.verbose {
		fmt.Fprintf(os.Stderr, "home: %s (from %s)\n", home, homeSource.Describe())
	}

	if err := checkExisting(home); err != nil {
		return err
	}

	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	isCI := os.Getenv("CI") == "true"
	wizard := isTTY && !opts.yes && !isCI

	if wizard {
		runInitWizard(cmd, opts)
	}

	// Resolve agent CLI (--agent flag > wizard choice > installed detection > config default).
	var agent jeff.AgentTool
	if opts.agent != "" {
		agent = jeff.AgentTool(opts.agent)
		if !agent.IsValid() {
			agent = jeff.AgentClaudeCode
		}
	} else {
		agent = defaultAgentFromInstalled()
	}

	// Resolve IDE.
	var ide jeff.IDE
	if opts.ide != "" {
		ide = jeff.IDE(opts.ide)
		if !ide.IsValid() {
			ide = jeff.IDEVSCode
		}
	}

	// Create directory structure.
	ensureDirs(home)

	// Build config.
	c := jeff.DefaultConfig()
	c.Home = home
	if agent.IsValid() {
		c.Agent = agent
	}
	if ide != "" {
		c.IDE = ide
	}
	if opts.gigPrefix != "" {
		c.GigPrefix = opts.gigPrefix
	}

	// Add repos from --repo flags.
	if !opts.noRepos && len(opts.repos) > 0 {
		for _, r := range opts.repos {
			name, url, _ := strings.Cut(r, "=")
			if name != "" {
				if c.Repos == nil {
					c.Repos = make(map[string]*jeff.RepoConfig)
				}
				c.Repos[name] = &jeff.RepoConfig{URL: url}
			}
		}
	}

	// Write config.
	if err := jeff.SaveConfig(&c); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Write CLAUDE.md.
	homePath := homePathLabel(home, opts.here)
	if err := jeffembed.WriteClaudeMD(home, homePath, false); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	// C2: Create context file aliases for all installed agents.
	var seededAgents int
	for _, a := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(a); p != nil {
			if aliases := p.ContextFileAliases(); len(aliases) > 0 {
				_ = jeffembed.CreateContextAliases(home, aliases)
			}
		}
	}

	// -----------------------------------------------------------------------
	// C5: Collect seeding failures instead of swallowing them.
	// -----------------------------------------------------------------------
	var seedErrs []error

	if err := jeffembed.EnsureGeminiSkillsAlias(home); err != nil {
		seedErrs = append(seedErrs, fmt.Errorf("alias .gemini/skills: %w", err))
	}
	if err := jeffembed.EnsureOpenCodeSkillsAlias(home); err != nil {
		seedErrs = append(seedErrs, fmt.Errorf("alias .opencode/skills: %w", err))
	}

	// Count seeded agents for output.
	for range jeff.RegisteredAgents() {
		seededAgents++
	}

	writeDefaults(home)

	if err := persona.SeedDefaults(home); err != nil {
		seedErrs = append(seedErrs, fmt.Errorf("seed personas: %w", err))
	}

	if err := skill.SeedDefaults(home); err != nil {
		seedErrs = append(seedErrs, fmt.Errorf("seed skills: %w", err))
	}

	// Seed persona-tagged embedded skills.
	if err := skill.SeedPersonaSkills(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed persona skills: %v\n", err)
	}

	// Install hooks.
	if err := syncHomeHooks(home, &c); err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}

	// Write global pointer.
	if err := jeff.WriteHomePointer(home); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}

	// Initialize memory subsystem.
	if err := memory.Initialize(home); err != nil {
		seedErrs = append(seedErrs, fmt.Errorf("init memory: %w", err))
	}

	// -----------------------------------------------------------------------
	// C3 + C4: Next-steps ladder + doctor reference.
	// -----------------------------------------------------------------------
	fmt.Fprintf(cmd.OutOrStdout(), "Initialized JEFF at %s\n\n", home)

	// C4: Quick dependency summary.
	quickDoctorSummary(cmd, home)

	// C3: Next-steps ladder.
	printNextSteps(cmd, opts.verbose, seededAgents)

	// Print seeding warnings (collected, not swallowed).
	if len(seedErrs) > 0 {
		fmt.Fprintln(cmd.ErrOrStderr())
		fmt.Fprintln(cmd.ErrOrStderr(), colorize(cYellow, "Warnings:"))
		for _, se := range seedErrs {
			fmt.Fprintf(cmd.ErrOrStderr(), "  • %v\n", se)
		}
		return &exitCode{code: 2, msg: "initialization completed with warnings"}
	}

	return nil
}

// ---------------------------------------------------------------------------
// C4: Quick doctor reference / summary.
// ---------------------------------------------------------------------------

func quickDoctorSummary(cmd *cobra.Command, home string) {
	out := cmd.OutOrStdout()

	deps := getDoctorDeps()
	var ok, missing int
	for _, d := range deps {
		if _, err := exec.LookPath(d.Binary); err != nil {
			missing++
		} else {
			ok++
		}
	}

	fmt.Fprintf(out, "Dependency check: %d ok, %d missing\n", ok, missing)
	if missing > 0 {
		fmt.Fprintln(out, "  Run "+colorize(cBold, "jeff doctor")+" for details and install commands")
	}
	fmt.Fprintln(out)
}

// ---------------------------------------------------------------------------
// C3: Next-steps ladder output.
// ---------------------------------------------------------------------------

func printNextSteps(cmd *cobra.Command, verbose bool, seededAgents int) {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, colorize(cBold, "Next steps — copy-paste friendly:"))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  1.  "+colorize(cBold, "jeff doctor")+"                    — verify all dependencies")
	fmt.Fprintln(out, "  2.  "+colorize(cBold, "jeff repo add <url>")+"         — register a codebase")
	fmt.Fprintln(out, "  3.  "+colorize(cBold, "jeff pickup <gig-id>")+"        — claim your first task")
	fmt.Fprintln(out, "  4.  "+colorize(cBold, "jeff ship")+"                    — push branches + create PRs")
	if seededAgents > 0 {
		fmt.Fprintln(out, "  5.  "+colorize(cBold, "jeff dashboard")+"                — open the TUI dashboard")
	}

	if verbose {
		fmt.Fprintln(out)
		fmt.Fprintln(out, colorize(cDim, "Directory structure:"))
		fmt.Fprintln(out, colorize(cDim, "  repos/      — register codebases with: jeff repo add <url>"))
		fmt.Fprintln(out, colorize(cDim, "  tasks/      — task workspaces created by: jeff pickup <gig-id>"))
		fmt.Fprintln(out, colorize(cDim, "  worktrees/  — git worktrees managed by: jeff worktree add"))
		fmt.Fprintln(out, colorize(cDim, "  exports/    — artifacts and generated files"))
		fmt.Fprintln(out, colorize(cDim, "  CLAUDE.md   — agent instructions (editable)"))
		fmt.Fprintln(out, colorize(cDim, "  hooks/      — session hooks (configure in jeff.json)"))
		fmt.Fprintln(out, colorize(cDim, "  memory/     — canonical memory (see docs/usage.md)"))
	}

	fmt.Fprintln(out)
}

// ---------------------------------------------------------------------------
// (Unchanged from before — helpers used by init and update.)
// ---------------------------------------------------------------------------

// runUpdate syncs an existing JEFF home — creates missing dirs, files, and hooks.
func runUpdate() error {
	home, err := jeff.ResolveHome()
	if err != nil {
		return fmt.Errorf("JEFF is not initialized. Run `jeff init` first")
	}
	if _, err := os.Stat(jeff.ConfigPath(home)); err != nil {
		return fmt.Errorf("JEFF is not initialized at %s. Run `jeff init` first", home)
	}

	c, err := jeff.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ensureDirs(home)

	writeDefaults(home)

	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			if aliases := p.ContextFileAliases(); len(aliases) > 0 {
				_ = jeffembed.CreateContextAliases(home, aliases)
			}
		}
	}

	if err := jeffembed.EnsureGeminiSkillsAlias(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .gemini/skills: %v\n", err)
	}
	if err := jeffembed.EnsureOpenCodeSkillsAlias(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .opencode/skills: %v\n", err)
	}

	if err := persona.SeedDefaults(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed personas: %v\n", err)
	}

	if err := skill.SeedDefaults(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed skills: %v\n", err)
	}

	if err := skill.SeedPersonaSkills(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed persona skills: %v\n", err)
	}

	// Sync hooks.
	if err := syncHomeHooks(home, c); err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}

	if err := jeff.WriteHomePointer(home); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}

	memReport, err := memory.Update(home)
	if err != nil {
		return fmt.Errorf("update memory: %w", err)
	}

	fmt.Printf("JEFF updated at %s (dirs, hooks, personas, providers, config synced)\n", home)
	fmt.Printf("  memory: %d new, %d skipped\n", len(memReport.Created), len(memReport.Skipped))
	if len(memReport.Migrations) > 0 {
		fmt.Println("  migration hints:")
		for _, h := range memReport.Migrations {
			fmt.Printf("    • %s\n", h)
		}
		fmt.Println("  Move legacy directories manually (source → dest under memory/...).")
	}
	return nil
}

// ensureDirs creates all expected directories under home (idempotent).
func ensureDirs(home string) {
	dirs := []string{
		home,
		filepath.Join(home, "repos"),
		filepath.Join(home, "tasks"),
		filepath.Join(home, "worktrees"),
		filepath.Join(home, "exports"),
		filepath.Join(home, "scripts"),
		filepath.Join(home, "projects"),
		filepath.Join(home, ".skills"),
		filepath.Join(home, ".personas"),
	}
	for _, d := range dirs {
		_ = os.MkdirAll(d, 0o755)
	}
	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			_ = p.EnsureHomeDirs(home)
		}
	}
}

// writeDefaults writes default files that don't already exist.
func writeDefaults(home string) {
	writeIfMissing(filepath.Join(home, ".skills", "skills.json"),
		"{\"$schema\":\"https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/skills.json\",\"skills\":{}}\n")
	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			_ = p.WriteHomeDefaults(home)
		}
	}
}

// writeIfMissing writes content to path only if the file doesn't exist.
func writeIfMissing(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}
