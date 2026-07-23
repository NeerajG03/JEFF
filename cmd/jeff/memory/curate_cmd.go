// curate_cmd.go — `jeff memory curate` — Worker C.
// Spawns marlowe non-interactively via the configured agent to process the
// queue + proposals and write canonical memory entries.
package memory

import (
	"fmt"
	"os"
	"path/filepath"

	jeff "github.com/NeerajG03/JEFF"
	jeffmemory "github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/skill"
	"github.com/spf13/cobra"
)

var curateFlags struct {
	persona   string
	skillPath string
}

var curateCmd = &cobra.Command{
	Use:   "curate",
	Short: "Run marlowe to consolidate proposals + queue into canonical memory",
	Long: `Reads JEFF_HOME/queue/sessions/*.json and JEFF_HOME/proposals/**, invokes
the marlowe persona via the configured agent (JEFF_MEMORY_CAN_ADD=1), and archives
processed entries. Marlowe writes canonical entries via 'jeff memory add'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}

		cfg, err := jeff.LoadConfig(home)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		provider := jeff.GetProvider(cfg.Agent)
		if provider == nil {
			return fmt.Errorf("no agent provider registered for %q — is jeff initialised?", cfg.Agent)
		}
		if provider.BuildCurateArgs("", jeff.LaunchOpts{}) == nil {
			return fmt.Errorf("agent %q does not support non-interactive curation", cfg.Agent)
		}

		skillContent, err := loadSkillContent(home, curateFlags.skillPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load curation skill: %v — continuing without it\n", err)
			skillContent = ""
		}

		runner := jeffmemory.ExecRunner{
			Command: provider.Command(),
			BuildArgs: func(prompt string) []string {
				return provider.BuildCurateArgs(prompt, jeff.LaunchOpts{})
			},
		}

		opts := jeffmemory.CurateOptions{
			Home:         home,
			Persona:      curateFlags.persona,
			Runner:       runner,
			SkillContent: skillContent,
		}

		report, err := jeffmemory.Curate(opts)
		if err != nil {
			return fmt.Errorf("curate: %w", err)
		}

		printCurateReport(cmd, report)
		return nil
	},
}

func init() {
	curateCmd.Flags().StringVarP(&curateFlags.persona, "persona", "p", "", "only process queue entries for this persona")
	curateCmd.Flags().StringVar(&curateFlags.skillPath, "skill", "", "path to curation SKILL.md (default: JEFF_HOME/.skills/curation/SKILL.md)")
}

// loadSkillContent reads the curation skill SKILL.md. Precedence:
//  1. --skill flag path (if set)
//  2. JEFF_HOME/.skills/curation/SKILL.md
//  3. Embedded fallback from skill package binary
func loadSkillContent(home, overridePath string) (string, error) {
	if overridePath != "" {
		data, err := os.ReadFile(overridePath)
		if err != nil {
			return "", fmt.Errorf("read --skill path: %w", err)
		}
		return string(data), nil
	}

	defaultPath := filepath.Join(home, ".skills", "curation", "SKILL.md")
	data, err := os.ReadFile(defaultPath)
	if err == nil {
		return string(data), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read skill file: %w", err)
	}

	return skill.CurationSkillContent()
}

func printCurateReport(cmd *cobra.Command, r jeffmemory.CurateReport) {
	cmd.Printf("\nCuration pass complete.\n")
	cmd.Printf("  Processed:   %d\n", r.Processed)
	cmd.Printf("  Accepted:    %d\n", r.Accepted)
	cmd.Printf("  Skipped:     %d  (dedupe hits)\n", r.Skipped)
	cmd.Printf("  Invalidated: %d\n", r.Invalidated)
	cmd.Printf("  Flagged:     %d  (need user review)\n", len(r.Flagged))

	if len(r.Flagged) > 0 {
		cmd.Printf("\nFlagged entries:\n")
		for _, name := range r.Flagged {
			cmd.Printf("  - %s\n", name)
		}
	}

	if len(r.Errors) > 0 {
		cmd.Printf("\nErrors:\n")
		for _, e := range r.Errors {
			cmd.Printf("  - %v\n", e)
		}
	}
}
