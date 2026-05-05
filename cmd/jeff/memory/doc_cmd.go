// doc_cmd.go — `jeff memory doc` prints the memory-system explainer.
package memory

import (
	"fmt"

	mem "github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

var docCmd = &cobra.Command{
	Use:   "doc",
	Short: "Print memory subsystem documentation",
	Long:  `Print a comprehensive explainer of the JEFF memory system.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), mem.Doc)
	},
}
