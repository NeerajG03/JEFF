// memory_cmd.go — root cobra command for `jeff memory`.
// Wired into cmd/jeff/main.go via memorycmd.Cmd.
package memory

import (
	"github.com/spf13/cobra"
)

// Cmd is the cobra root command for `jeff memory`. Subcommands are registered
// in init() below; stub subcommands return "not yet implemented" until their
// owning worker fills them in.
var Cmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage JEFF memory (propose, curate, list, show, status, …)",
	Long: `JEFF memory subsystem v1.

Workers (jenko/schmidt/eric/...) propose memories via "jeff memory propose".
The curator (marlowe) processes proposals via "jeff memory curate" and is the
only writer to JEFF_HOME/memory/**.

See exports/memory-research/specs/EPIC.md for the full design.`,
}

func init() {
	Cmd.AddCommand(proposeCmd)
	Cmd.AddCommand(addCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(showCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(curateCmd)
	Cmd.AddCommand(diffCmd)
	Cmd.AddCommand(disableCmd)
	Cmd.AddCommand(docCmd)
	// Worker A: hooks + injection + suppression.
	Cmd.AddCommand(sessionStartCmd)
	Cmd.AddCommand(sessionEndCmd)
	// Worker E: init / update / migrate.
	Cmd.AddCommand(migrateCmd)
}
