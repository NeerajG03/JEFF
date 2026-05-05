// disable_cmd.go — `jeff memory disable [--confirm]` toggles the memory disabled flag.
package memory

import (
	"fmt"
	"io"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/spf13/cobra"
)

var disableConfirm bool

var disableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Toggle JEFF memory disabled flag (advisory — use --confirm to write)",
	Long: `Toggle the JEFF memory disabled flag in jeff.json.

Without --confirm, prints instructions for disabling memory.
With --confirm, writes {"memory":{"disabled":true}} to jeff.json (toggle: if
already disabled, re-enables it).

Advisory: workers honor the flag by skipping the memory addendum and not calling
'jeff memory propose'. The orchestrator skips memory hooks when the flag is set.
Canonical memory is not deleted; it can be re-enabled at any time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}
		return runDisable(cmd.OutOrStdout(), home, disableConfirm)
	},
}

func init() {
	disableCmd.Flags().BoolVar(&disableConfirm, "confirm", false, "Actually toggle the disable flag (writes to jeff.json)")
}

func runDisable(out io.Writer, home string, confirm bool) error {
	if !confirm {
		fmt.Fprintln(out, `To disable JEFF memory, run one of:

  jeff memory disable --confirm
    Writes {"memory":{"disabled":true}} to jeff.json.
    Run again to toggle back on.

  export JEFF_MEMORY_DISABLE=1
    Disables for this shell session only (does not persist).

When disabled, workers receive no memory addendum and will not call
'jeff memory propose'. The SessionEnd hook is skipped. Canonical memory
in JEFF_HOME/memory/ is untouched and can be re-enabled at any time.`)
		return nil
	}

	cfg, err := jeff.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Memory != nil && cfg.Memory.Disabled {
		cfg.Memory.Disabled = false
		if err := jeff.SaveConfig(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Fprintln(out, "Memory re-enabled. Workers will receive the memory addendum on next pickup.")
		return nil
	}

	if cfg.Memory == nil {
		cfg.Memory = &jeff.MemoryConfig{}
	}
	cfg.Memory.Disabled = true
	if err := jeff.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintln(out, "Memory disabled. Run 'jeff memory disable --confirm' again to re-enable.")
	return nil
}
