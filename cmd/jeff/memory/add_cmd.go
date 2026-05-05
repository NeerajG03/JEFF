// add_cmd.go — stub: Worker B fills in.
// See exports/memory-research/specs/B-capture.md
//
// `add` is gated by JEFF_MEMORY_CAN_ADD=1 (only set in marlowe sessions).
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a canonical memory entry (curator only — gated by JEFF_MEMORY_CAN_ADD)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker B will fill this in")
	},
}
