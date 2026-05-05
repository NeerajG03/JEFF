// diff_cmd.go — stub: Worker D fills in.
// See exports/memory-research/specs/D-introspect.md
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Diff a memory entry against its prior (superseded) version",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker D will fill this in")
	},
}
