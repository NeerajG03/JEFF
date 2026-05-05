// curate_cmd.go — stub: Worker C fills in.
// See exports/memory-research/specs/C-curate.md
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var curateCmd = &cobra.Command{
	Use:   "curate",
	Short: "Run marlowe to consolidate proposals + queue into canonical memory",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker C will fill this in")
	},
}
