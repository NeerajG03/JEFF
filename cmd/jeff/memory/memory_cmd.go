// memory_cmd.go — root cobra command for `jeff memory`.
package memory

import (
	"github.com/spf13/cobra"
)

// Cmd is the cobra root command for `jeff memory` (hook infrastructure only).
var Cmd = &cobra.Command{
	Use:   "memory",
	Short: "Memory subsystem — session hook commands",
	Long:  "Memory commands used by session hooks. Not intended for direct interactive use.",
}

func init() {
	// Worker A: hooks + injection + suppression.
	Cmd.AddCommand(sessionStartCmd)
	Cmd.AddCommand(sessionEndCmd)
}
