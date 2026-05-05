// status_cmd.go — stub: Worker D fills in.
// See exports/memory-research/specs/D-introspect.md
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory subsystem status (queue depth, proposals pending, etc.)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker D will fill this in")
	},
}
