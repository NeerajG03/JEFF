// disable_cmd.go — stub: Worker D fills in.
// See exports/memory-research/specs/D-introspect.md
//
// `disable` soft-invalidates a canonical entry (sets valid_to). The actual
// write is delegated to the curate path so single-writer invariants hold.
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var disableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Soft-invalidate a memory entry (sets valid_to)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker D will fill this in")
	},
}
