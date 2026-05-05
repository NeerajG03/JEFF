// list_cmd.go — stub: Worker D fills in.
// See exports/memory-research/specs/D-introspect.md
package memory

import (
	"errors"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List canonical memory entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("not yet implemented: Worker D will fill this in")
	},
}
