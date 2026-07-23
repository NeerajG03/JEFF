// memory_cmd.go — root cobra command for `jeff memory`.
package memory

import (
	"github.com/spf13/cobra"
)

// Cmd is the cobra root command for `jeff memory`. Subcommands are registered
// in init() below.
var Cmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage JEFF memory (propose, curate, list, show, add, disable)",
	Long: `JEFF memory subsystem v1.

Workers (jenko/schmidt/eric/...) propose memories via "jeff memory propose".
The curator (marlowe) processes proposals via "jeff memory curate" and is the
only writer to JEFF_HOME/memory/**.`,
}

func init() {
	Cmd.AddCommand(proposeCmd)
	Cmd.AddCommand(addCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(showCmd)
	Cmd.AddCommand(curateCmd)
	Cmd.AddCommand(disableCmd)
	// Worker A: hooks + injection + suppression.
	Cmd.AddCommand(sessionStartCmd)
	Cmd.AddCommand(sessionEndCmd)
}
