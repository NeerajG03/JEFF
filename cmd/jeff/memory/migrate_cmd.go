// migrate_cmd.go — `jeff memory migrate` command.
// Moves legacy memory/learnings layout into the v1 memory tree.
package memory

import (
	"fmt"
	"os"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate legacy memory/learnings layout into the v1 memory tree",
	Long: `Migrate moves old-style memory files into the new canonical layout:

  personas/<x>/memory/MEMORY.md    → memory/personas/<x>/semantic/INDEX.md
  personas/<x>/memory/<detail>.md  → memory/personas/<x>/semantic/<detail>.md
  learnings/<repo>/INDEX.md        → memory/repos/<repo>/semantic/INDEX.md
  learnings/<repo>/<detail>.md     → memory/repos/<repo>/semantic/<detail>.md

Old files are moved to archive/migration-YYYYMMDD/ (never deleted).

Use --dry-run to preview what would change without writing anything.
Use --confirm to apply the migration.`,
	RunE: runMigrate,
}

func init() {
	migrateCmd.Flags().Bool("dry-run", false, "Preview changes without writing")
	migrateCmd.Flags().Bool("confirm", false, "Apply the migration (required unless --dry-run)")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	confirm, _ := cmd.Flags().GetBool("confirm")

	if !dryRun && !confirm {
		return fmt.Errorf("use --dry-run to preview or --confirm to apply the migration")
	}

	home, err := jeff.ResolveHome()
	if err != nil {
		return fmt.Errorf("JEFF is not initialized. Run `jeff init` first")
	}
	if _, err := os.Stat(jeff.ConfigPath(home)); err != nil {
		return fmt.Errorf("JEFF is not initialized at %s. Run `jeff init` first", home)
	}

	report, err := memory.Migrate(home, dryRun)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if dryRun {
		fmt.Println("Dry run — no files written.")
	}

	if len(report.Moved) == 0 && len(report.Skipped) == 0 {
		fmt.Println("Nothing to migrate.")
		return nil
	}

	if len(report.Moved) > 0 {
		if dryRun {
			fmt.Printf("Would move %d file(s):\n", len(report.Moved))
		} else {
			fmt.Printf("Moved %d file(s):\n", len(report.Moved))
		}
		for _, m := range report.Moved {
			fmt.Printf("  %s\n", m)
		}
	}

	if len(report.Skipped) > 0 {
		fmt.Printf("Skipped %d file(s) (unrecognized pattern — handle manually):\n", len(report.Skipped))
		for _, s := range report.Skipped {
			fmt.Printf("  %s\n", s)
		}
	}

	if len(report.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "%d error(s) during migration:\n", len(report.Errors))
		for _, e := range report.Errors {
			fmt.Fprintf(os.Stderr, "  %v\n", e)
		}
		return fmt.Errorf("migration completed with %d error(s)", len(report.Errors))
	}

	if !dryRun {
		fmt.Println("Migration complete. Run `jeff memory list` to inspect the new layout.")
	} else {
		fmt.Println("Run `jeff memory migrate --confirm` to apply.")
	}

	return nil
}
