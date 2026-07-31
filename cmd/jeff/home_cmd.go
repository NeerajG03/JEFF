package main

import (
	"fmt"
	"os"
	"path/filepath"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/internal/homepath"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/skill"
	"github.com/spf13/cobra"
)

// homeCmd exposes the home lifecycle. `jeff home` explains resolution (which layer
// won, and why); `jeff home use` is the ONLY command besides `jeff init` permitted
// to write the pointer file. Read paths never write it — a one-shot
// `JEFF_HOME=/tmp/x jeff status` must not repoint the persistent default.
func homeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "home",
		Short: "Show the resolved JEFF home and where it came from",
		Long: `Show the resolved JEFF home and which layer of the chain decided it.

Resolution precedence (read path, every command):
  1. $JEFF_HOME          per-process / per-tmux-session override
  2. ~/.config/jeff/home  the persistent install record
  3. ~/.jeff              bootstrap default

Only ` + "`jeff init`" + ` and ` + "`jeff home use`" + ` write the pointer file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			home, source, err := jeff.ResolveHomeWithSource()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "home:    %s\n", home)
			fmt.Fprintf(out, "source:  %s\n", source.Describe())

			ptr, perr := jeff.PointerPath()
			if perr == nil {
				recorded := "(not set)"
				if data, err := os.ReadFile(ptr); err == nil {
					recorded = trimLine(string(data))
				}
				fmt.Fprintf(out, "pointer: %s → %s\n", ptr, recorded)
			}

			if jeff.IsHomeInitialized(home) {
				fmt.Fprintln(out, "status:  initialized")
			} else {
				fmt.Fprintln(out, "status:  NOT initialized (run `jeff init`)")
			}
			return nil
		},
	}
	cmd.AddCommand(homeUseCmd())
	return cmd
}

func homeUseCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "use <path>",
		Short: "Point JEFF at an existing home (records it and repairs stale paths)",
		Long: `Record <path> as the JEFF home in ~/.config/jeff/home, then repair anything
inside it that still refers to a previous location.

This is the supported way to relocate a home:

    mv ~/.jeff /new/place/jeff
    jeff home use /new/place/jeff

Registries store persona/skill locations relative to the home, so they travel with
it. This command additionally rewrites any leftover absolute entries and
regenerates the per-agent settings, whose hook commands are absolute by necessity.

Note that $JEFF_HOME, if set, still overrides the pointer for the current shell.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			target, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			info, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("%s: %w", target, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", target)
			}
			if !jeff.IsHomeInitialized(target) && !force {
				return fmt.Errorf("%s has no jeff.json — not an initialized JEFF home\nRun `jeff init --home %s` to create one, or pass --force to point at it anyway", target, target)
			}

			if err := jeff.WriteHomePointer(target); err != nil {
				return fmt.Errorf("write home pointer: %w", err)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Home pointer set to %s\n", target)

			if !jeff.IsHomeInitialized(target) {
				fmt.Fprintln(out, "  (uninitialized — skipping path repair; run `jeff init --update`)")
				return nil
			}

			repaired, err := repairHomePaths(target)
			if err != nil {
				return err
			}
			for _, line := range repaired {
				fmt.Fprintf(out, "  %s\n", line)
			}

			if env := os.Getenv(jeff.EnvHome); env != "" && env != target {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"\nWarning: $%s is set to %s and overrides the pointer in this shell.\n  Unset it (or export %s=%s) for the new home to take effect here.\n",
					jeff.EnvHome, env, jeff.EnvHome, target)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Point at the directory even if it holds no jeff.json")
	return cmd
}

// repairHomePaths rewrites state inside home that still names a previous location.
// Two categories exist, handled differently:
//
//	registries (personas.json, skills.json) — relative by construction now, so a
//	  plain Load+Save rewrites any legacy absolute entry. Entries whose target no
//	  longer exists but which have a counterpart inside this home are repointed.
//	per-agent settings (.claude/settings.json, .gemini/settings.json) — hook
//	  commands must be absolute to be executable, so these are REGENERATED from
//	  the current home rather than rewritten.
func repairHomePaths(home string) ([]string, error) {
	var report []string

	cfg, err := jeff.LoadConfig(home)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// --- personas.json ---
	pc, err := persona.LoadPersonas(home)
	if err != nil {
		return nil, fmt.Errorf("load personas: %w", err)
	}
	personasFixed := 0
	for name, entry := range pc.Personas {
		if fixed, changed := relocateInto(home, persona.DefaultPersonasDir(home), entry.Location, name); changed {
			entry.Location = fixed
			personasFixed++
		}
	}
	if err := persona.SavePersonas(home, pc); err != nil {
		return nil, fmt.Errorf("save personas: %w", err)
	}
	report = append(report, fmt.Sprintf("personas.json rewritten home-relative (%d location(s) repointed)", personasFixed))

	// --- skills.json ---
	sc, err := skill.LoadSkills(home)
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	skillsFixed := 0
	for name, entry := range sc.Skills {
		if fixed, changed := relocateInto(home, skill.DefaultSkillsDir(home), entry.Location, name); changed {
			entry.Location = fixed
			skillsFixed++
		}
	}
	if err := skill.SaveSkills(home, sc); err != nil {
		return nil, fmt.Errorf("save skills: %w", err)
	}
	report = append(report, fmt.Sprintf("skills.json rewritten home-relative (%d location(s) repointed)", skillsFixed))

	// --- per-agent settings + hook scripts ---
	if err := syncHomeHooks(home, cfg); err != nil {
		return nil, fmt.Errorf("regenerate agent settings/hooks: %w", err)
	}
	report = append(report, "agent settings + hook scripts regenerated for this home")

	return report, nil
}

// relocateInto repairs a single stored location. A location that still resolves is
// left alone. One that does not, but whose basename exists under this home's
// managed dir, is repointed there — the state a home lands in when it was moved
// while an absolute path was still recorded.
func relocateInto(home, managedDir, location, name string) (string, bool) {
	if location == "" {
		return location, false
	}
	if _, err := os.Stat(location); err == nil {
		return location, false
	}
	candidate := filepath.Join(managedDir, name)
	if _, err := os.Stat(candidate); err != nil {
		return location, false // genuinely external or genuinely gone — leave it
	}
	if homepath.Inside(home, location) {
		return candidate, false // already home-relative; nothing user-visible changed
	}
	return candidate, true
}

// trimLine strips a trailing newline from a single-line file's contents.
func trimLine(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	if s == "" {
		return "(empty)"
	}
	return s
}
