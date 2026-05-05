// status_cmd.go — `jeff memory status`.
package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	mem "github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory subsystem status (queue depth, proposals pending, entry counts)",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}
		return runStatus(cmd.OutOrStdout(), home, statusJSON)
	},
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}

type statusResult struct {
	QueueDepth    int    `json:"queue_depth"`
	Proposals     int    `json:"proposals_pending"`
	LastCurate    string `json:"last_curate,omitempty"`
	CanonicalTotal int   `json:"canonical_total"`
	ByScope       struct {
		Persona int `json:"persona"`
		Repo    int `json:"repo"`
		Project int `json:"project"`
	} `json:"by_scope"`
	Superseded int `json:"superseded"`
	Flagged    int `json:"flagged"`
}

func runStatus(out io.Writer, home string, asJSON bool) error {
	queueEntries, err := mem.ListQueueEntries(home)
	if err != nil {
		return fmt.Errorf("list queue: %w", err)
	}

	proposals, err := mem.ListProposals(home, "", "")
	if err != nil {
		return fmt.Errorf("list proposals: %w", err)
	}

	// Canonical accepted entries.
	acceptedEntries, err := mem.ListEntries(home, mem.EntryFilter{Status: "accepted"})
	if err != nil {
		return fmt.Errorf("list accepted: %w", err)
	}

	// Superseded entries.
	supersededEntries, err := mem.ListEntries(home, mem.EntryFilter{Status: "superseded"})
	if err != nil {
		return fmt.Errorf("list superseded: %w", err)
	}

	var personaCount, repoCount, projectCount, flaggedCount int
	for _, e := range acceptedEntries {
		parts := strings.SplitN(e.Scope, ":", 2)
		if len(parts) == 2 {
			switch parts[0] {
			case "persona":
				personaCount++
			case "repo":
				repoCount++
			case "project":
				projectCount++
			}
		}
		if e.FM.Provenance == "review-required" {
			flaggedCount++
		}
	}

	canonicalTotal := personaCount + repoCount + projectCount
	lastCurate := lastCuratedTime(home)

	if asJSON {
		r := statusResult{
			QueueDepth:    len(queueEntries),
			Proposals:     len(proposals),
			CanonicalTotal: canonicalTotal,
			Superseded:    len(supersededEntries),
			Flagged:       flaggedCount,
		}
		r.ByScope.Persona = personaCount
		r.ByScope.Repo = repoCount
		r.ByScope.Project = projectCount
		if lastCurate != nil {
			r.LastCurate = lastCurate.Format(time.RFC3339)
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}

	fmt.Fprintln(out, "Memory status")
	fmt.Fprintln(out, strings.Repeat("─", 40))

	fmt.Fprintf(out, "%-14s %d sessions pending consolidation\n", "Queue:", len(queueEntries))
	fmt.Fprintf(out, "%-14s %d entries pending curation\n", "Proposals:", len(proposals))

	if lastCurate != nil {
		ago := time.Since(*lastCurate).Round(time.Minute)
		fmt.Fprintf(out, "%-14s %s (%s ago)\n", "Last curate:", lastCurate.Format(time.RFC3339), formatDuration(ago))
	} else {
		fmt.Fprintf(out, "%-14s never\n", "Last curate:")
	}

	fmt.Fprintf(out, "%-14s %d entries (%d persona, %d repo, %d project)\n",
		"Canonical:", canonicalTotal, personaCount, repoCount, projectCount)
	fmt.Fprintf(out, "%-14s %d entries (history preserved)\n", "Superseded:", len(supersededEntries))
	fmt.Fprintf(out, "%-14s %d entries pending review\n", "Flagged:", flaggedCount)

	return nil
}

// lastCuratedTime reads the optional JEFF_HOME/memory/.last-curated marker file.
// Worker C writes this file after each successful curate run.
func lastCuratedTime(home string) *time.Time {
	data, err := os.ReadFile(filepath.Join(mem.MemoryRoot(home), ".last-curated"))
	if err != nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return nil
	}
	return &t
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "< 1 minute"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
}
