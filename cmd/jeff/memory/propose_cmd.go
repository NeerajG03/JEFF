// propose_cmd.go — `jeff memory propose` implementation.
// Open to all personas; writes to proposals/<persona>/<task>/<slug>.md.
// See exports/memory-research/specs/B-capture.md
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

// kebabRe matches valid kebab-case slugs: lowercase, digits, hyphens (no
// leading/trailing/consecutive hyphens).
var kebabRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// validateMemoryName returns an error if name is not a valid memory slug.
// Shared by propose_cmd and add_cmd (same package).
func validateMemoryName(name string) error {
	if len(name) > 64 {
		return fmt.Errorf("name %q exceeds 64 characters", name)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("name %q must not contain '/'", name)
	}
	if !kebabRe.MatchString(name) {
		return fmt.Errorf("name %q must be lowercase kebab-case (a-z, 0-9, hyphens; no leading/trailing/consecutive hyphens)", name)
	}
	return nil
}

var proposeCmd = newProposeCmd()

func newProposeCmd() *cobra.Command {
	var (
		flagName        string
		flagType        string
		flagDescription string
		flagBody        string
		flagPersona     string
		flagTask        string
		flagJSON        bool
		flagForce       bool
	)

	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Propose a memory entry (writes to proposals/ — workers only)",
		Long: `Propose a memory entry for later curation by marlowe.

The proposal is written to proposals/<persona>/<task>/<slug>.md under JEFF_HOME.
marlowe (Worker C) processes proposals and writes to canonical memory.

Persona and task default to $JEFF_PERSONA and $JEFF_TASK_ID when not provided.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			persona := flagPersona
			if persona == "" {
				persona = os.Getenv("JEFF_PERSONA")
			}
			if persona == "" {
				return fmt.Errorf("persona is required: pass --persona or set JEFF_PERSONA")
			}

			task := flagTask
			if task == "" {
				task = os.Getenv("JEFF_TASK_ID")
			}
			if task == "" {
				return fmt.Errorf("task is required: pass --task or set JEFF_TASK_ID")
			}

			if err := validateMemoryName(flagName); err != nil {
				return err
			}

			memType, err := memory.ParseMemoryType(flagType)
			if err != nil {
				return err
			}

			home, err := jeff.ResolveHome()
			if err != nil {
				return fmt.Errorf("resolve JEFF_HOME: %w", err)
			}

			// Name collision guard.
			taskDir := memory.ProposalsTaskPath(home, persona, task)
			existing := filepath.Join(taskDir, flagName+".md")
			if _, statErr := os.Stat(existing); statErr == nil && !flagForce {
				return fmt.Errorf("proposal %q already exists at %s\nuse --force to overwrite", flagName, existing)
			}

			fm := memory.Frontmatter{
				Name:        flagName,
				Description: flagDescription,
				Type:        memType,
			}

			p, err := memory.WriteProposal(home, persona, task, fm, flagBody)
			if err != nil {
				return fmt.Errorf("write proposal: %w", err)
			}

			if flagJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(p)
			}

			rel, relErr := filepath.Rel(home, p.Path)
			if relErr != nil {
				rel = p.Path
			}
			fmt.Fprintf(cmd.OutOrStdout(), "proposed: %s\n", rel)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagName, "name", "", "memory slug (kebab-case, ≤64 chars)")
	cmd.Flags().StringVar(&flagType, "type", "", "memory type (user|feedback|project|reference)")
	cmd.Flags().StringVar(&flagDescription, "description", "", "one-line description")
	cmd.Flags().StringVar(&flagBody, "body", "", "full body (multiline ok via shell quoting)")
	cmd.Flags().StringVar(&flagPersona, "persona", "", "proposing persona (default: $JEFF_PERSONA)")
	cmd.Flags().StringVar(&flagTask, "task", "", "task ID (default: $JEFF_TASK_ID)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "output proposal as JSON")
	cmd.Flags().BoolVar(&flagForce, "force", false, "overwrite existing proposal with same name")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("description")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}
