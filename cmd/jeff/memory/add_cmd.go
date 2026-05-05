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

			scopeKind, scopeName, err := parseScope(flagScope)
			if err != nil {
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

			scopePath := resolveScopePath(home, scopeKind, scopeName)

			filePath, err := resolveEntryPath(scopePath, bucket, flagName)
			if err != nil {
				return err
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

			f, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("create entry: %w", err)
			}
			if writeErr := memory.WriteCanonical(f, fm, flagBody); writeErr != nil {
				f.Close()
				return writeErr
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close entry: %w", err)
			}

			if err := updateIndex(filepath.Dir(filePath), flagName, flagDescription, memType); err != nil {
				return fmt.Errorf("update index: %w", err)
			}

			rel, relErr := filepath.Rel(home, filePath)
			if relErr != nil {
				rel = filePath
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

func resolveScopePath(home string, kind memory.ScopeKind, name string) string {
	switch kind {
	case memory.ScopePersona:
		return memory.PersonaScopePath(home, name)
	case memory.ScopeRepo:
		return memory.RepoScopePath(home, name)
	case memory.ScopeProject:
		return memory.ProjectScopePath(home, name)
	default: // ScopeOrchestrator
		return memory.OrchestratorScopePath(home)
	}
}

// resolveEntryPath returns the absolute path for the canonical entry file and
// ensures its parent directory exists.
func resolveEntryPath(scopePath string, bucket memory.Bucket, slug string) (string, error) {
	bucketPath := memory.BucketPath(scopePath, bucket)
	if bucket == memory.BucketCore {
		// core.md lives directly in the scope dir (BucketPath returns <scope>/core).
		filePath := bucketPath + ".md"
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return "", fmt.Errorf("mkdir scope: %w", err)
		}
		return filePath, nil
	}
	if err := os.MkdirAll(bucketPath, 0o755); err != nil {
		return "", fmt.Errorf("mkdir bucket: %w", err)
	}
	return filepath.Join(bucketPath, slug+".md"), nil
}

// updateIndex appends an entry line to INDEX.md in dir (creating it if absent).
func updateIndex(dir, slug, description string, t memory.MemoryType) error {
	path := filepath.Join(dir, "INDEX.md")

	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = strings.TrimRight(string(data), "\n")
	} else {
		existing = "# Memory Index"
	}

	entry := fmt.Sprintf("- **%s** (`%s`): %s", slug, t, description)
	return os.WriteFile(path, []byte(existing+"\n"+entry+"\n"), 0o644)
}
