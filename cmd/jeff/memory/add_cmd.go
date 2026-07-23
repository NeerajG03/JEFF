// add_cmd.go — `jeff memory add` implementation.
// Gated by JEFF_MEMORY_CAN_ADD=1; only the marlowe curator session sets this.
// See exports/memory-research/specs/B-capture.md
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

var addCmd = newAddCmd()

func newAddCmd() *cobra.Command {
	var (
		flagName        string
		flagType        string
		flagDescription string
		flagBody        string
		flagPersona     string
		flagTask        string
		flagScope       string
		flagBucket      string
		flagSupersede   string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a canonical memory entry (curator only — gated by JEFF_MEMORY_CAN_ADD)",
		Long: `Write a canonical memory entry directly to JEFF_HOME/memory/.

Restricted to the marlowe curator session (JEFF_MEMORY_CAN_ADD=1 must be set).
Workers should use 'jeff memory propose' instead.

Scope format: persona:<name>  repo:<name>  project:<name>  orchestrator
Bucket:       core | procedural | semantic | episodic`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Permission check — enforced before any other work.
			if os.Getenv("JEFF_MEMORY_CAN_ADD") != "1" {
				return fmt.Errorf("error: jeff memory add is restricted to the marlowe curator session.\nUse 'jeff memory propose' instead.")
			}

			persona := flagPersona
			if persona == "" {
				persona = os.Getenv("JEFF_PERSONA")
			}
			task := flagTask
			if task == "" {
				task = os.Getenv("JEFF_TASK_ID")
			}

			if err := validateMemoryName(flagName); err != nil {
				return err
			}

			memType, err := memory.ParseMemoryType(flagType)
			if err != nil {
				return err
			}

			if _, _, err := parseScope(flagScope); err != nil {
				return err
			}

			bucket := memory.Bucket(flagBucket)
			if !bucket.Valid() {
				return fmt.Errorf("invalid bucket %q (want core|procedural|semantic|episodic)", flagBucket)
			}

			home, err := jeff.ResolveHome()
			if err != nil {
				return fmt.Errorf("resolve JEFF_HOME: %w", err)
			}

			fm := memory.CanonicalFrontmatter{
				Frontmatter: memory.Frontmatter{
					Name:        flagName,
					Description: flagDescription,
					Type:        memType,
				},
				Status:     "accepted",
				Scope:      flagScope,
				ValidFrom:  time.Now().UTC(),
				Provenance: "trusted",
				Source: memory.Source{
					Persona: persona,
					Task:    task,
					Trigger: "curator-add",
				},
			}

			var entry memory.Entry
			if flagSupersede != "" {
				entry, err = memory.Supersede(home, flagSupersede, fm, flagBody)
				if err != nil {
					return fmt.Errorf("supersede: %w", err)
				}
			} else {
				entry, err = memory.WriteCanonical(home, flagScope, flagBucket, fm, flagBody)
				if err != nil {
					return fmt.Errorf("write canonical: %w", err)
				}
				if err := memory.UpdateIndex(home, flagScope, flagBucket); err != nil {
					return fmt.Errorf("update index: %w", err)
				}
			}

			rel, relErr := filepath.Rel(home, entry.Path)
			if relErr != nil {
				rel = entry.Path
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added: %s\n", rel)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagName, "name", "", "memory slug (kebab-case, ≤64 chars)")
	cmd.Flags().StringVar(&flagType, "type", "", "memory type (user|feedback|project|reference)")
	cmd.Flags().StringVar(&flagDescription, "description", "", "one-line description")
	cmd.Flags().StringVar(&flagBody, "body", "", "full body (multiline ok via shell quoting)")
	cmd.Flags().StringVar(&flagPersona, "persona", "", "source persona (default: $JEFF_PERSONA)")
	cmd.Flags().StringVar(&flagTask, "task", "", "source task ID (default: $JEFF_TASK_ID)")
	cmd.Flags().StringVar(&flagScope, "scope", "", "canonical scope: persona:<name>|repo:<name>|project:<name>|orchestrator")
	cmd.Flags().StringVar(&flagBucket, "bucket", "", "memory bucket (core|procedural|semantic|episodic)")
	cmd.Flags().StringVar(&flagSupersede, "supersede", "", "path to existing entry to supersede (calls Supersede instead of WriteCanonical)")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("description")
	_ = cmd.MarkFlagRequired("body")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("bucket")

	return cmd
}

// parseScope splits "persona:jenko", "repo:jeff", "project:foo", or "orchestrator"
// into (ScopeKind, name). Returns an error for unrecognized formats.
func parseScope(scope string) (memory.ScopeKind, string, error) {
	if scope == "" {
		return "", "", fmt.Errorf("scope is required")
	}
	if scope == "orchestrator" {
		return memory.ScopeOrchestrator, "", nil
	}
	idx := strings.Index(scope, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid scope %q: expected kind:name (e.g. persona:jenko, repo:jeff, project:foo, orchestrator)", scope)
	}
	kind := memory.ScopeKind(scope[:idx])
	name := scope[idx+1:]
	if name == "" {
		return "", "", fmt.Errorf("invalid scope %q: name after ':' is empty", scope)
	}
	if !kind.Valid() {
		return "", "", fmt.Errorf("invalid scope kind %q (want persona|repo|project|orchestrator)", kind)
	}
	return kind, name, nil
}
