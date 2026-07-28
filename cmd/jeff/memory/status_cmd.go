// status_cmd.go — `jeff memory status`: queue depth, counts, last curation.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	mem "github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

type statusCounts struct {
	proposals int
	canonical int
	queue     int
}

type scopeCount struct {
	label string
	count int
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory subsystem status — proposals, canonical entries, last curation",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}
		return runStatus(cmd, home)
	},
}

func runStatus(cmd *cobra.Command, home string) error {
	// Proposal count.
	proposals, err := mem.ListProposals(home, "", "")
	if err != nil {
		return fmt.Errorf("list proposals: %w", err)
	}

	// Queue entries.
	queueDir := mem.QueueSessionsRoot(home)
	queueCount := 0
	if ents, err := os.ReadDir(queueDir); err == nil {
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				queueCount++
			}
		}
	}

	// Canonical entries — walk memory/.
	entries, err := mem.ListEntries(home, mem.EntryFilter{})
	if err != nil {
		return fmt.Errorf("list entries: %w", err)
	}

	// Per-scope breakdown.
	scopeMap := map[string]int{}
	for _, e := range entries {
		scopeMap[e.Scope]++
	}

	// Per-bucket breakdown.
	bucketMap := map[string]int{}
	for _, e := range entries {
		bucketMap[string(e.Bucket)]++
	}

	// Last curation stamp.
	lastCurated := "never"
	stampPath := filepath.Join(mem.MemoryRoot(home), ".last-curated")
	if data, err := os.ReadFile(stampPath); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			lastCurated = t.Format("2006-01-02 15:04 UTC")
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Memory status for %s\n\n", home)
	fmt.Fprintf(cmd.OutOrStdout(), "  Last curation:   %s\n", lastCurated)
	fmt.Fprintf(cmd.OutOrStdout(), "  Proposals:       %d pending\n", len(proposals))
	fmt.Fprintf(cmd.OutOrStdout(), "  Queue:           %d session(s) pending\n", queueCount)
	fmt.Fprintf(cmd.OutOrStdout(), "  Canonical:       %d entry(ies)\n", len(entries))
	fmt.Fprintln(cmd.OutOrStdout())

	if len(bucketMap) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  By bucket:")
		for _, b := range mem.Buckets {
			if c := bucketMap[string(b)]; c > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "    %-14s %d\n", string(b)+":", c)
			}
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if len(scopeMap) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  By scope:")
		for label, count := range scopeMap {
			fmt.Fprintf(cmd.OutOrStdout(), "    %-24s %d\n", label+":", count)
		}
	}

	return nil
}
