// propose_cmd.go — stub: Worker B fills in.
// See exports/memory-research/specs/B-capture.md
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var proposeCmd = &cobra.Command{
	Use:   "propose",
	Short: "Propose a memory entry (writes to proposals/ — workers only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker B will fill this in")
	},
}
